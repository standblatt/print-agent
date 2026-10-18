package main

import (
	"bytes"
	"testing"

	"github.com/standblatt/print-agent/internal/winspool"
)

func TestTestPage(t *testing.T) {
	pdf := testPage("Standblatt test page", "Brother (QL-700) ü €")
	if !bytes.Contains(pdf, []byte(`(Brother \(QL-700\) `+"\xfc"+` ?) Tj`)) {
		t.Fatalf("escaping: %s", pdf)
	}
	doc, err := (&winspool.Renderer{}).Open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	pages, err := doc.Pages()
	if err != nil || len(pages) != 1 || int(pages[0][0]+0.5) != 255 || int(pages[0][1]+0.5) != 82 {
		t.Fatalf("PDFium reads %v %v, want one page of 90 × 29 mm (255 × 82 pt)", pages, err)
	}
}
