package main

import (
	"bytes"
	"fmt"
	"strings"
)

// testPage returns a one-page PDF of a 90 × 29 mm label, the size of the server's test page, with a frame 2 mm inside
// the edge and the given lines; on an office printer with noscale it prints in a corner of the sheet.
func testPage(lines ...string) []byte {
	const mm = 72 / 25.4
	w, h := 90*mm, 29*mm
	var c strings.Builder
	fmt.Fprintf(&c, "0.5 w %.2f %.2f %.2f %.2f re S\n", 2*mm, 2*mm, w-4*mm, h-4*mm)
	for i, l := range lines {
		size := 9.0
		if i == 0 {
			size = 12
		}
		fmt.Fprintf(&c, "BT /F1 %.0f Tf %.2f %.2f Td (%s) Tj ET\n", size, 5*mm, h-9*mm-float64(i)*5.5*mm, pdfString(l))
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", w, h),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", c.Len(), c.String()),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// pdfString escapes s for a PDF string in WinAnsi encoding; characters outside Latin-1 become "?".
func pdfString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r > 0xff || r >= 0x7f && r < 0xa0:
			b.WriteByte('?')
		default:
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}
