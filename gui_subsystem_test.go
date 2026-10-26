package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A Windows "-H=windowsgui" executable has NO valid stdout/stderr handles.
// v2.7.0 passed every other test but failed here (PDFium never started and the
// legacy interpreter was used). This test runs the engine in a child process
// whose stdout/stderr are closed files, reproducing that condition.
func TestEngineStartsWithoutConsoleHandles(t *testing.T) {
	if os.Getenv("TRIVEX_CHILD_NO_STDIO") == "1" {
		dead, _ := os.Open(os.DevNull)
		dead.Close()
		os.Stdout, os.Stderr = dead, dead
		b, _ := BuildSyntheticVariant("rotate90")
		x, err := ExtractDocument(b)
		if err != nil || x.Legacy {
			msg := "legacy"
			if x != nil {
				msg = x.EngineError
			}
			if err != nil {
				msg = err.Error()
			}
			os.WriteFile(os.Getenv("TRIVEX_CHILD_OUT"), []byte("FAIL "+msg), 0600)
			os.Exit(0)
		}
		a, _, _, _ := AnalyzeExtraction(x)
		rep := RunSelfTest()
		res := "OK"
		if len(a.Wires) != 7 || !rep.Pass {
			res = "FAIL records/selftest"
		}
		os.WriteFile(os.Getenv("TRIVEX_CHILD_OUT"), []byte(res), 0600)
		os.Exit(0)
	}
	out := t.TempDir() + "/result"
	cmd := exec.Command(os.Args[0], "-test.run=TestEngineStartsWithoutConsoleHandles", "-test.count=1")
	cmd.Env = append(os.Environ(), "TRIVEX_CHILD_NO_STDIO=1", "TRIVEX_CHILD_OUT="+out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("child: %v", err)
	}
	r, _ := os.ReadFile(out)
	if !strings.HasPrefix(string(r), "OK") {
		t.Fatalf("engine without console handles: %s", r)
	}
}
