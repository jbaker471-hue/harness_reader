package main

import (
	"os"
	"strings"
	"testing"
)

func TestReferencePDFValidationRejectsBannersAndBadFiles(t *testing.T) {
	banner := []byte("<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.0 Transitional//EN\"><html><title>Defense Logistics Agency Warning Banner</title>")
	denied := []byte("<HTML><HEAD><TITLE>Access Denied</TITLE></HEAD><BODY>You don't have permission</BODY></HTML>")
	for name, b := range map[string][]byte{"banner": banner, "denied": denied, "bom html": append([]byte("\xef\xbb\xbf  \r\n"), denied...)} {
		c := ValidateReferencePDF(b)
		if c.OK || c.Status != refDocManualRequired || !strings.Contains(c.Reason, "not a PDF") {
			t.Fatalf("%s: %+v", name, c)
		}
	}
	for name, b := range map[string][]byte{"empty": {}, "text": []byte("hello"), "json": []byte(`{"error":"login"}`)} {
		if c := ValidateReferencePDF(b); c.OK || c.Status != refDocRejected {
			t.Fatalf("%s: %+v", name, c)
		}
	}
	// the signature alone is not enough: it must parse
	if c := ValidateReferencePDF([]byte("%PDF-1.7\nthis is not a real document")); c.OK || c.Status != refDocRejected || !strings.Contains(c.Reason, "did not parse") {
		t.Fatalf("truncated PDF accepted: %+v", c)
	}
}

func TestReferencePDFValidationAcceptsRealPDF(t *testing.T) {
	b, err := os.ReadFile("testdata/SYNTHETIC_RECOGNITION_TEST.pdf")
	if err != nil {
		t.Fatal(err)
	}
	c := ValidateReferencePDF(b)
	if !c.OK || c.Status != refDocAccepted || c.Pages < 1 || len(c.SHA256) != 64 || c.Bytes != len(b) {
		t.Fatalf("%+v", c)
	}
}
