package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Reference-document intake. A URL that ends in .pdf is not necessarily a PDF: government
// consent banners, login pages and access-denied pages come back as HTML with a 200 or 403.
// A file is accepted only when it carries the PDF signature, is not an HTML page, and
// parses with the PDF engine. Anything else is reported, never partly accepted.

const (
	refDocAccepted       = "ACCEPTED"
	refDocManualRequired = "MANUAL_DOWNLOAD_REQUIRED"
	refDocRejected       = "REJECTED"
)

type ReferenceDocCheck struct {
	Status string `json:"status"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
	Bytes  int    `json:"bytes"`
	Pages  int    `json:"pages,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

var htmlStarts = [][]byte{[]byte("<!doctype html"), []byte("<html"), []byte("<head"), []byte("<?xml")}

func looksLikeHTML(b []byte) bool {
	head := bytes.ToLower(bytes.TrimLeft(b[:min(len(b), 1024)], " \t\r\n\xef\xbb\xbf"))
	for _, s := range htmlStarts {
		if bytes.HasPrefix(head, s) {
			return true
		}
	}
	return false
}

// ValidateReferencePDF checks downloaded or user-supplied bytes. It never stores them.
func ValidateReferencePDF(b []byte) ReferenceDocCheck {
	out := ReferenceDocCheck{Status: refDocRejected, Bytes: len(b)}
	switch {
	case len(b) == 0:
		out.Reason = "The file is empty."
		return out
	case looksLikeHTML(b):
		out.Status = refDocManualRequired
		out.Reason = "This is a web page, not a PDF. A warning or consent banner, login page or access-denied page was saved instead of the document. Download it in a browser and import that file."
		return out
	case !bytes.HasPrefix(bytes.TrimLeft(b[:min(len(b), 1024)], " \t\r\n\x00"), []byte("%PDF-")) && !isPDF(b):
		out.Reason = "The file does not start with the PDF signature (%PDF-)."
		return out
	case len(b) > maxPDFSize:
		out.Reason = "The PDF is larger than the supported size."
		return out
	}
	sum := sha256.Sum256(b)
	out.SHA256 = hex.EncodeToString(sum[:])
	x, err := ExtractDocument(b)
	if err != nil {
		out.Reason = "The PDF did not parse: " + err.Error()
		return out
	}
	out.Pages = len(x.Data.Pages)
	if out.Pages == 0 {
		out.Reason = "The PDF parsed but has no readable pages."
		return out
	}
	if strings.TrimSpace(x.EngineError) != "" {
		out.Reason = "The PDF engine reported an error: " + x.EngineError
		return out
	}
	out.Status, out.OK = refDocAccepted, true
	out.Reason = "Valid PDF. It has been checked only; coordinates are not read from it until a reviewed table is entered."
	return out
}
