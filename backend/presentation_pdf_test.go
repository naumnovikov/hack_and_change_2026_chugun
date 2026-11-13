package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestPresentationPDFStructureAndCyrillic(t *testing.T) {
	tasks := testTasks()
	pdf, err := renderPresentation(testPresentation(tasks), tasks)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("invalid PDF header/trailer")
	}
	if n := len(regexp.MustCompile(`/Type /Page\b`).FindAll(pdf, -1)); n != 5 {
		t.Fatalf("PDF pages=%d want 5", n)
	}
	if !bytes.Contains(pdf, []byte("/FontFile2")) || !bytes.Contains(pdf, []byte("/ToUnicode")) {
		t.Fatal("missing embedded TrueType font or Unicode map")
	}
	// Verify xref points to actual indirect objects rather than just checking a signature.
	start := regexp.MustCompile(`startxref\s+(\d+)`).FindSubmatch(pdf)
	if len(start) != 2 {
		t.Fatal("missing xref")
	}
	offset, _ := strconv.Atoi(string(start[1]))
	if offset >= len(pdf) || !bytes.HasPrefix(pdf[offset:], []byte("xref")) {
		t.Fatal("broken xref offset")
	}
	entries := regexp.MustCompile(`(?m)^(\d{10}) 00000 n `).FindAllSubmatch(pdf, -1)
	if len(entries) == 0 {
		t.Fatal("empty xref")
	}
	for _, entry := range entries {
		off, _ := strconv.Atoi(string(entry[1]))
		if off >= len(pdf) || !regexp.MustCompile(`^\d+ 0 obj`).Match(pdf[off:]) {
			t.Fatal("xref references missing object")
		}
	}
	// With Identity-H and compression disabled Cyrillic must be present in page
	// streams as UTF-16BE, not as replacement '?' characters or a metadata-only title.
	var utf16hex strings.Builder
	for _, r := range []rune("Демо-черновик") {
		fmt.Fprintf(&utf16hex, "%04x", r)
	}
	encoded, _ := hex.DecodeString(utf16hex.String())
	if !bytes.Contains(pdf, encoded) {
		t.Fatal("Cyrillic page text missing")
	}
	if !strings.Contains(goFontLicense, "Bigelow & Holmes") || !strings.Contains(fpdfLicense, "MIT License") {
		t.Fatal("license notices missing")
	}
	// Optional artifact for independent parser/render inspection; never needed by CI.
	if path := os.Getenv("TEST_PRESENTATION_PDF"); path != "" {
		if err = os.WriteFile(path, pdf, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPresentationPDFRejectsInvalidDraft(t *testing.T) {
	if _, err := renderPresentation(presentationDraft{}, testTasks()); err == nil {
		t.Fatal("invalid draft rendered")
	}
}
