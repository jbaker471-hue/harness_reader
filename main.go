package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed assets/ui.html
var embeddedUI embed.FS

const maxPDFSize = 100 << 20

// ---------------------------------------------------------------------------
// Window lifecycle
//
// The backend must stop when the user closes the app window with its X, so
// that no process keeps the installation folder locked. Three independent
// signals end the process, whichever comes first:
//
//  1. The dedicated Edge process (own temporary profile) exits.
//  2. The page reports it is closing (pagehide) and no heartbeat follows
//     within a short grace period (a reload sends a new heartbeat).
//  3. Heartbeats stop for heartbeatTimeout (window killed, browser crashed,
//     or the launcher handed the window to another browser process).
//
// On exit the Edge process tree is terminated and its profile is deleted.

const (
	heartbeatTimeout = 150 * time.Second // hidden/minimized windows throttle timers to ~1/min
	closingGrace     = 4 * time.Second
	firstContactWait = 3 * time.Minute
)

type lifecycle struct {
	mu        sync.Mutex
	lastBeat  time.Time
	beats     int
	closingAt time.Time
}

func (l *lifecycle) beat() {
	l.mu.Lock()
	l.lastBeat = time.Now()
	l.beats++
	l.closingAt = time.Time{}
	l.mu.Unlock()
}
func (l *lifecycle) connected() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.beats > 0
}
func (l *lifecycle) closing() {
	l.mu.Lock()
	l.closingAt = time.Now()
	l.mu.Unlock()
}

// expired reports why the UI is considered gone, or "". While our own Edge
// process is alive, only an explicit closing signal counts: a minimized or
// background window may legitimately stop sending heartbeats.
func (l *lifecycle) expired(started time.Time, edgeAlive bool) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if !l.closingAt.IsZero() && now.Sub(l.closingAt) > closingGrace {
		return "application window closed"
	}
	if edgeAlive {
		return ""
	}
	if l.beats == 0 {
		if now.Sub(started) > firstContactWait {
			return "application window never connected"
		}
		return ""
	}
	if now.Sub(l.lastBeat) > heartbeatTimeout {
		return "application window stopped responding"
	}
	return ""
}

type appWindow struct {
	cmd        *exec.Cmd
	profileDir string
	exited     chan struct{}
	startedAt  time.Time
}

// launchWindow opens the UI in a dedicated Microsoft Edge app window with an
// isolated temporary profile. If Edge is not available, the default browser
// is used and heartbeats alone control shutdown.
func launchWindow(url string) (*appWindow, error) {
	if runtime.GOOS != "windows" {
		if os.Getenv("DISPLAY") != "" {
			return &appWindow{startedAt: time.Now()}, exec.Command("xdg-open", url).Start()
		}
		return &appWindow{startedAt: time.Now()}, nil
	}
	profileDir, err := os.MkdirTemp("", "trivex-edge-app-*")
	if err != nil {
		return nil, err
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		args := []string{"--app=" + url, "--new-window", "--window-size=1440,950", "--no-first-run", "--no-default-browser-check",
			"--disable-background-mode", "--disable-sync", "--disable-features=msEdgeStartupBoost,EdgeCollections,msUndersideButton",
			"--no-service-autorun", "--user-data-dir=" + profileDir}
		cmd := exec.Command(candidate, args...)
		if err := cmd.Start(); err != nil {
			continue
		}
		w := &appWindow{cmd: cmd, profileDir: profileDir, exited: make(chan struct{}), startedAt: time.Now()}
		go func() {
			if err := cmd.Wait(); err != nil {
				log.Println("Edge window process ended:", err)
			}
			close(w.exited)
		}()
		return w, nil
	}
	os.RemoveAll(profileDir)
	// Fallback: default browser. Shutdown relies on heartbeat/closing signals.
	log.Println("Microsoft Edge not found; opening the default browser (heartbeat-controlled shutdown)")
	if err := openDefaultBrowser(url); err != nil {
		return nil, fmt.Errorf("no browser could be started: %w", err)
	}
	return &appWindow{startedAt: time.Now()}, nil
}

func (w *appWindow) shutdown() {
	if w == nil {
		return
	}
	if w.cmd != nil && w.cmd.Process != nil {
		select {
		case <-w.exited:
		default:
			killProcessTree(w.cmd.Process.Pid)
			select {
			case <-w.exited:
			case <-time.After(5 * time.Second):
			}
		}
	}
	if w.profileDir != "" {
		// Edge child processes can hold profile files for a moment after exit.
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(w.profileDir); err == nil {
				return
			}
			time.Sleep(300 * time.Millisecond)
		}
		log.Println("Temporary Edge profile could not be fully removed; it is in the user TEMP folder and contains no drawing data")
	}
}

func main() {
	logPath := filepath.Join(os.TempDir(), "Trivex_Harness_Auditor_startup.log")
	if f, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); e == nil {
		defer f.Close()
		log.SetOutput(f)
	}
	if err := run(); err != nil {
		log.Println("Startup failed:", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log.Println("Backend stopped cleanly")
}

func queryInt(r *http.Request, k string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil {
		return v
	}
	return def
}

func run() error {
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(k[:])
	ui, err := embeddedUI.ReadFile("assets/ui.html")
	if err != nil {
		return err
	}
	if err := validateReferenceLibraries(); err != nil {
		// Loud failure: a malformed tooling library must never silently drop rows.
		log.Println("Reference library validation failed:", err)
	}
	page := strings.ReplaceAll(string(ui), "__SESSION_TOKEN__", token)
	page = strings.ReplaceAll(page, "__APP_VERSION__", AppVersion)
	go func() {
		if err := WarmEngine(); err != nil {
			log.Println("PDFium warm-up failed:", err)
		}
	}()
	mux := http.NewServeMux()
	exitRequested := make(chan string, 4)
	life := &lifecycle{}
	secure := func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Header.Get("X-Trivex-Token") != token {
			http.Error(w, "Unauthorized local request", http.StatusForbidden)
			return false
		}
		if r.Header.Get("Origin") != "" && !strings.HasPrefix(r.Header.Get("Origin"), "http://127.0.0.1:") {
			http.Error(w, "Unexpected origin", http.StatusForbidden)
			return false
		}
		return true
	}
	post := func(w http.ResponseWriter, r *http.Request) bool {
		if !secure(w, r) {
			return false
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", http.StatusMethodNotAllowed)
			return false
		}
		return true
	}
	jsonAnswer := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(v)
	}
	readPDF := func(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
		r.Body = http.MaxBytesReader(w, r.Body, maxPDFSize)
		b, e := io.ReadAll(r.Body)
		if e != nil {
			http.Error(w, "PDF exceeds size limit", 413)
			return nil, false
		}
		return b, true
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self' blob: data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-src blob:; connect-src 'self'; img-src 'self' blob: data:")
		io.WriteString(w, page)
	})
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		status := "ok"
		if err := WarmEngine(); err != nil {
			status = err.Error()
		}
		jsonAnswer(w, map[string]string{"version": AppVersion, "engine": EngineName(), "engineStatus": status, "go": runtime.Version(), "os": runtime.GOOS + "/" + runtime.GOARCH})
	})
	mux.HandleFunc("/api/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		life.beat()
		jsonAnswer(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/closing", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		life.closing()
		jsonAnswer(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/exit", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"closing":true}`)
		select {
		case exitRequested <- "Exit application requested":
		default:
		}
	})
	mux.HandleFunc("/api/parse", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		b, ok := readPDF(w, r)
		if !ok {
			return
		}
		if !isPDF(b) {
			http.Error(w, "Expected PDF file", 422)
			return
		}
		x, e := ExtractDocument(b)
		if e != nil {
			http.Error(w, "PDF interpretation failed: "+e.Error(), 422)
			return
		}
		current.set(b, x)
		v, e := viewFromExtraction(x)
		if e != nil {
			http.Error(w, "PDF analysis failed: "+e.Error(), 422)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		current.clear()
		jsonAnswer(w, map[string]bool{"cleared": true})
	})
	mux.HandleFunc("/api/page-image", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		pg := queryInt(r, "page", 1)
		scale, _ := strconv.ParseFloat(r.URL.Query().Get("scale"), 64)
		if scale <= 0 || scale > 4 {
			scale = 1.5
		}
		img, err := current.render(pg, scale)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		data, err := encodePNG(img, 0)
		if err != nil {
			http.Error(w, "render encode failed", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(data)
	})
	mux.HandleFunc("/api/page-layout", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		_, x := current.get()
		pg := queryInt(r, "page", 1)
		if x == nil || pg < 1 || pg > len(x.Layouts) {
			http.Error(w, "No such page in the loaded drawing", 404)
			return
		}
		pl := x.Layouts[pg-1]
		labels := labelHits([]PageLayout{pl})
		jsonAnswer(w, map[string]any{"page": pl.Page, "width": pl.Width, "height": pl.Height, "rotation": pl.Rotation, "words": pl.Words, "images": pl.Images, "circles": pl.Circles, "segments": pl.Segments, "insertDiagrams": insertDiagrams([]PageLayout{pl}, labels), "notes": pl.Notes, "vectorDetailTruncated": pl.VectorDetailTruncated})
	})
	mux.HandleFunc("/api/region", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var req RegionRequest
		if e := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); e != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		_, x := current.get()
		if x == nil {
			http.Error(w, "No drawing is loaded", 409)
			return
		}
		res, e := analyzeRegion(x, req)
		if e != nil {
			http.Error(w, e.Error(), 422)
			return
		}
		jsonAnswer(w, res)
	})
	mux.HandleFunc("/api/embedded-images", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		b, ok := readPDF(w, r)
		if !ok {
			return
		}
		if len(b) > 0 {
			if !isPDF(b) {
				http.Error(w, "Expected PDF", 422)
				return
			}
			if _, x := current.get(); x == nil || x.SHA256 != sha256Hex(b) {
				x, e := ExtractDocument(b)
				if e != nil {
					http.Error(w, "PDF image extraction failed", 422)
					return
				}
				current.set(b, x)
			}
		}
		images, e := galleryFromSession(current)
		if e != nil {
			http.Error(w, "Picture extraction failed: "+e.Error(), 422)
			return
		}
		jsonAnswer(w, images)
	})
	mux.HandleFunc("/api/raw-image", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		img, err := current.rawImage(queryInt(r, "page", 1), queryInt(r, "id", 1))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		data, err := encodePNG(img, 640)
		if err != nil {
			http.Error(w, "encode failed", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(data)
	})
	mux.HandleFunc("/api/build-plan", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var req PlanRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
			http.Error(w, "Invalid build plan JSON", http.StatusBadRequest)
			return
		}
		if len(req.Connectors) > 1000 || req.BuildQty > 100000 {
			http.Error(w, "Build plan exceeds supported limits", http.StatusBadRequest)
			return
		}
		result, err := MakeBuildPlan(req)
		if err != nil {
			http.Error(w, "Build plan reference unavailable: "+err.Error(), 500)
			return
		}
		jsonAnswer(w, result)
	})
	mux.HandleFunc("/api/accessory-check", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var req AccRequest
		if e := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); e != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		res, e := CheckAccessories(req)
		if e != nil {
			http.Error(w, "Accessory reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, res)
	})
	mux.HandleFunc("/api/layouts", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		learned.mu.RLock()
		path, lerr := learned.path, learned.loadErr
		learned.mu.RUnlock()
		jsonAnswer(w, map[string]any{"entries": learned.list(), "path": path, "error": lerr})
	})
	mux.HandleFunc("/api/layouts/learn", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var req struct {
			Kind    string   `json:"kind"`
			Headers []string `json:"headers"`
			Roles   []string `json:"roles"`
		}
		if e := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&req); e != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		n, e := learned.learn(req.Kind, headingWordsFromGrid(req.Headers, req.Roles))
		if e != nil {
			http.Error(w, "Could not save learned layout: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, map[string]int{"learned": n})
	})
	mux.HandleFunc("/api/layouts/forget", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var req struct {
			Label string `json:"label"`
		}
		if e := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); e != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		if e := learned.forget(req.Label); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonAnswer(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/layouts/import", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		var list []LearnedHeading
		if e := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&list); e != nil {
			http.Error(w, "Not a Trivex layout file", 400)
			return
		}
		n, e := learned.importList(list)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonAnswer(w, map[string]int{"imported": n})
	})
	mux.HandleFunc("/api/reanalyze", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		_, x := current.get()
		if x == nil {
			http.Error(w, "No drawing is loaded", 409)
			return
		}
		v, e := viewFromExtraction(x)
		if e != nil {
			http.Error(w, e.Error(), 422)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/probe", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		_, x := current.get()
		if x == nil {
			http.Error(w, "No drawing is loaded", 409)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, RecognitionProbe(x))
	})
	mux.HandleFunc("/api/self-test", func(w http.ResponseWriter, r *http.Request) {
		if !post(w, r) {
			return
		}
		jsonAnswer(w, RunSelfTest())
	})
	mux.HandleFunc("/api/resolve", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		var v struct {
			Part string `json:"part"`
		}
		if e := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&v); e != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		part := strings.ToUpper(strings.TrimSpace(v.Part))
		n, st := getLayout(part)
		inferred := ResolveConnectorContacts(part)
		labels := []string{}
		status := "catalog-count-only"
		source := "Amphenol insert availability chart; exact cavity IDs require separate verification"
		if g, ok := guideInventory(part); ok {
			labels = g.IDs
			status = "verified-layout"
			source = g.Source
			st = "Guide cavity-ID inventory; physical face orientation and complete wiring coverage still require review"
		}
		jsonAnswer(w, map[string]any{"contactCandidates": inferred.Contacts, "contactMappingStatus": inferred.Status, "contactMappingWarning": inferred.Warning, "contactMappingSource": inferred.Source, "part": part, "positionCount": n, "status": status, "contactSex": func() string {
			if inferred.ContactSex != "" {
				return inferred.ContactSex
			}
			if strings.HasSuffix(part, "PA") {
				return "PIN"
			}
			if strings.HasSuffix(part, "SA") {
				return "SOCKET"
			}
			return "UNKNOWN"
		}(), "source": source, "family": "MIL-DTL-38999", "warning": st, "labels": labels})
	})
	mux.HandleFunc("/api/tooling", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		v, e := crimpReference()
		if e != nil {
			http.Error(w, "Tooling reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/specialty-tooling", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		v, e := specialtyReference()
		if e != nil {
			http.Error(w, "Specialty reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/assembly-library", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		v, e := accessoryReference()
		if e != nil {
			http.Error(w, "Accessory reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/wire-cable-library", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		v, e := wireCableReference()
		if e != nil {
			http.Error(w, "Wire/cable reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/wire-decode", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		part := r.URL.Query().Get("part")
		if len(part) > 80 {
			http.Error(w, "Part number too long", http.StatusBadRequest)
			return
		}
		text := r.URL.Query().Get("description")
		if len(text) > 300 {
			http.Error(w, "Description too long", http.StatusBadRequest)
			return
		}
		v, e := DecodeWireLine(part, text)
		if e != nil {
			http.Error(w, "Wire specification reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	mux.HandleFunc("/api/connector-family", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		part := r.URL.Query().Get("part")
		if len(part) > 80 {
			http.Error(w, "Part number too long", http.StatusBadRequest)
			return
		}
		jsonAnswer(w, DecodeConnectorFamily(part))
	})
	mux.HandleFunc("/api/tool-equivalence-library", func(w http.ResponseWriter, r *http.Request) {
		if !secure(w, r) {
			return
		}
		v, e := toolEquivalenceReference()
		if e != nil {
			http.Error(w, "Tool-equivalence reference unavailable: "+e.Error(), 500)
			return
		}
		jsonAnswer(w, v)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 8 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 5 * time.Minute}
	go func() {
		if e := server.Serve(ln); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Println("Server error:", e)
		}
	}()
	log.Printf("Trivex Auditor v%s listening locally on %s", AppVersion, ln.Addr().String())
	if os.Getenv("TRIVEX_NO_WINDOW") == "1" {
		// Headless mode for automated tests only: print the URL and token.
		info := fmt.Sprintf("TRIVEX_URL=%s\nTRIVEX_TOKEN=%s\n", url, token)
		if f := os.Getenv("TRIVEX_URL_FILE"); f != "" {
			os.WriteFile(f, []byte(info), 0600) // console-less test runs
		} else {
			fmt.Print(info)
		}
	}
	var win *appWindow
	if os.Getenv("TRIVEX_NO_WINDOW") != "1" {
		win, err = launchWindow(url)
		if err != nil {
			log.Println("Unable to start an app window:", err)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
			return err
		}
	} else {
		win = &appWindow{startedAt: time.Now()}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	var edgeExited <-chan struct{}
	if win.exited != nil {
		edgeExited = win.exited
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	reason := ""
	for reason == "" {
		select {
		case <-edgeExited:
			// A launcher that hands off to another Edge process exits almost
			// immediately; in that case keep running on heartbeats instead.
			if time.Since(win.startedAt) < 8*time.Second && !life.connected() {
				log.Println("Edge launcher exited early; shutdown now follows the window heartbeat")
				edgeExited = nil
				continue
			}
			reason = "application window closed (Edge process ended)"
		case reason = <-exitRequested:
		case <-signals:
			reason = "terminated by system signal"
		case <-ticker.C:
			if os.Getenv("TRIVEX_NO_WINDOW") == "1" && os.Getenv("TRIVEX_TEST_HEARTBEAT") != "1" {
				continue
			}
			reason = life.expired(win.startedAt, edgeExited != nil)
		}
	}
	log.Println("Stopping backend:", reason)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	current.clear()
	win.shutdown()
	CloseEngine()
	return nil
}
