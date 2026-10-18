package winspool

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/standblatt/print-agent/internal/agent"
)

// pdfPrinter is built into Windows and can write into a file without a dialog.
const pdfPrinter = "Microsoft Print to PDF"

func TestDevmodeSize(t *testing.T) {
	if s := unsafe.Sizeof(devmode{}); s != 220 {
		t.Fatalf("sizeof(DEVMODEW) = %d, want 220", s)
	}
}

func TestPrinters(t *testing.T) {
	ps, err := (&Winspool{Renderer: renderer}).Printers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(ps, func(p agent.Printer) bool { return p.Name == pdfPrinter })
	if i < 0 {
		t.Fatalf("%q not listed: %+v", pdfPrinter, ps)
	}
	var paper, scale *agent.Option
	for k, o := range ps[i].Options {
		switch o.Name {
		case "paper":
			paper = &ps[i].Options[k]
		case "scale":
			scale = &ps[i].Options[k]
		}
	}
	if paper == nil || !slices.Contains(paper.Values, "A5") || paper.Default == "" || scale == nil || scale.Default != "noscale" {
		t.Fatalf("options: %+v", ps[i].Options)
	}
}

// printToPDF prints the fixture on the PDF printer with opts and returns the page sizes of the result.
func printToPDF(t *testing.T, ctx context.Context, opts map[string]string) ([][2]float64, error) {
	pdf, err := os.ReadFile("testdata/a5-2pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	p, err := find(pdfPrinter)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Translate(opts, caps(p))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.pdf")
	if err := (&Winspool{Renderer: renderer}).print(ctx, pdfPrinter, "Standblatt test", pdf, s, out); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := renderer.Open(b)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	return doc.Pages()
}

func TestPrint(t *testing.T) {
	for _, c := range []struct {
		paper string
		w, h  float64
	}{{"A5", 419.5, 595.3}, {"A4", 595.3, 841.9}} {
		sizes, err := printToPDF(t, context.Background(), map[string]string{"paper": c.paper})
		if err != nil {
			t.Fatal(err)
		}
		if len(sizes) != 2 {
			t.Fatalf("%s: %d pages", c.paper, len(sizes))
		}
		for _, s := range sizes {
			if math.Abs(s[0]-c.w) > 1 || math.Abs(s[1]-c.h) > 1 {
				t.Fatalf("%s: page size %v", c.paper, s)
			}
		}
	}
}

func TestPrintErrors(t *testing.T) {
	w := &Winspool{Renderer: renderer}
	ctx := context.Background()
	if err := w.Print(ctx, &agent.Job{Printer: "No such printer"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown printer: %v", err)
	}
	if err := w.Print(ctx, &agent.Job{Printer: pdfPrinter, Options: map[string]string{"bin": "Tray 9"}}); err == nil || !strings.Contains(err.Error(), `no tray "Tray 9"`) {
		t.Errorf("unknown tray: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := printToPDF(t, cancelled, nil); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("cancelled job: %v", err)
	}
}
