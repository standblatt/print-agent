package winspool

import (
	"os"
	"testing"
)

// renderer is shared by the tests of the package, so PDFium starts once.
var renderer = &Renderer{}

func TestRender(t *testing.T) {
	pdf, err := os.ReadFile("testdata/a5-2pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := renderer.Open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	sizes, err := doc.Pages()
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 2 || int(sizes[0][0]) != 419 || int(sizes[0][1]) != 595 {
		t.Fatalf("pages: %v", sizes)
	}
	// A5 at 72 dpi: 420×595 px; the diagonals end in the corners, the middle of the left edge is blank and the
	// frame 5 mm (14 px) inside the edge is drawn.
	img, done, err := doc.Render(1, 420, 595)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if b := img.Bounds(); b.Dx() != 420 || b.Dy() != 595 {
		t.Fatalf("size %v", b)
	}
	dark := func(x, y int) bool { c := img.RGBAAt(x, y); return c.R < 160 && c.A == 255 }
	white := func(x, y int) bool {
		c := img.RGBAAt(x, y)
		return c.R == 255 && c.G == 255 && c.B == 255 && c.A == 255
	}
	if !dark(0, 0) || !dark(419, 594) || !white(3, 297) || !dark(14, 297) {
		t.Fatalf("corner %v %v, edge %v, frame %v", img.RGBAAt(0, 0), img.RGBAAt(419, 594), img.RGBAAt(3, 297), img.RGBAAt(14, 297))
	}
}

func TestRenderNotAPDF(t *testing.T) {
	if _, err := renderer.Open([]byte("not a pdf")); err == nil {
		t.Fatal("garbage opened as a PDF")
	}
}
