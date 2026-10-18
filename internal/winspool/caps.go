// Package winspool is the print backend for Windows: it lists the printers of the spooler with their trays and
// papers, renders PDFs with PDFium (WebAssembly, no CGO) and prints the pages through GDI.
package winspool

import (
	"fmt"
	"slices"

	"github.com/standblatt/print-agent/internal/agent"
)

// Entry is a tray or paper: the ID the DEVMODE takes and the name the driver shows.
type Entry struct {
	ID   int16
	Name string
}

// Caps is what a printer driver reports about a printer, with the defaults of its DEVMODE.
type Caps struct {
	Bins, Papers  []Entry
	Duplex, Color bool
	Default       Settings
}

// Settings is a job's options translated for the DEVMODE; zero values keep the driver's default.
type Settings struct {
	Bin, Paper int16
	Duplex     int16 // DMDUP_SIMPLEX 1, DMDUP_VERTICAL 2 (long edge), DMDUP_HORIZONTAL 3 (short edge)
	Color      int16 // DMCOLOR_MONOCHROME 1, DMCOLOR_COLOR 2
	Scale      string
}

var (
	duplexValues = []string{"simplex", "duplexlong", "duplexshort"}
	scaleValues  = []string{"noscale", "shrink", "fit"}
	colorValues  = []string{"color", "monochrome"}
)

// unique drops entries whose name came earlier: drivers repeat names (and DC_BINNAMES cuts them at 24 characters),
// and the server picks by name.
func unique(es []Entry) []Entry {
	var out []Entry
	for _, e := range es {
		if !slices.ContainsFunc(out, func(o Entry) bool { return o.Name == e.Name }) {
			out = append(out, e)
		}
	}
	return out
}

// names returns the names of the entries and the name of the entry with ID def.
func names(es []Entry, def int16) ([]string, string) {
	vals, d := []string{}, ""
	for _, e := range es {
		vals = append(vals, e.Name)
		if e.ID == def {
			d = e.Name
		}
	}
	return vals, d
}

// Options returns the options the server shows for a printer: bin and paper by name, duplex and color only when
// the printer can, and the scaling.
func Options(c Caps) []agent.Option {
	var opts []agent.Option
	if bins := unique(c.Bins); len(bins) > 0 {
		vals, def := names(bins, c.Default.Bin)
		opts = append(opts, agent.Option{Name: "bin", Label: "Tray", Values: vals, Default: def})
	}
	if papers := unique(c.Papers); len(papers) > 0 {
		vals, def := names(papers, c.Default.Paper)
		opts = append(opts, agent.Option{Name: "paper", Label: "Paper", Values: vals, Default: def})
	}
	if c.Duplex {
		def := "simplex"
		if c.Default.Duplex >= 1 && c.Default.Duplex <= 3 {
			def = duplexValues[c.Default.Duplex-1]
		}
		opts = append(opts, agent.Option{Name: "duplex", Label: "Two-sided", Values: duplexValues, Default: def})
	}
	opts = append(opts, agent.Option{Name: "scale", Label: "Scaling", Values: scaleValues, Default: "noscale"})
	if c.Color {
		def := "color"
		if c.Default.Color == 1 {
			def = "monochrome"
		}
		opts = append(opts, agent.Option{Name: "color", Label: "Colour", Values: colorValues, Default: def})
	}
	return opts
}

// id returns the ID of the entry named name.
func id(es []Entry, key, name string) (int16, error) {
	for _, e := range unique(es) {
		if e.Name == name {
			return e.ID, nil
		}
	}
	return 0, fmt.Errorf("the printer has no %s %q", key, name)
}

// Translate turns a job's options into DEVMODE settings for a printer; unknown options or values are errors, so a
// job never prints with settings the admin did not choose.
func Translate(opts map[string]string, c Caps) (Settings, error) {
	s := Settings{Scale: "noscale"}
	for k, v := range opts {
		var err error
		switch k {
		case "bin":
			s.Bin, err = id(c.Bins, "tray", v)
		case "paper":
			s.Paper, err = id(c.Papers, "paper", v)
		case "duplex":
			if i := slices.Index(duplexValues, v); i >= 0 && c.Duplex {
				s.Duplex = int16(i + 1)
			} else {
				err = fmt.Errorf("the printer cannot print %q", v)
			}
		case "color":
			if i := slices.Index(colorValues, v); i >= 0 && c.Color {
				s.Color = int16(2 - i)
			} else {
				err = fmt.Errorf("the printer cannot print in %q", v)
			}
		case "scale":
			if !slices.Contains(scaleValues, v) {
				err = fmt.Errorf("unknown scaling %q", v)
			}
			s.Scale = v
		default:
			err = fmt.Errorf("unknown option %q", k)
		}
		if err != nil {
			return Settings{}, err
		}
	}
	return s, nil
}

// Device is the geometry of a printer DC in device pixels (GetDeviceCaps).
type Device struct {
	DPIX, DPIY       int // LOGPIXELSX/Y
	OffX, OffY       int // PHYSICALOFFSETX/Y: where the printable area starts on the paper
	HorzRes, VertRes int // HORZRES/VERTRES: the printable area
}

// Rect is a target rectangle relative to the printable area, as StretchDIBits takes it.
type Rect struct{ X, Y, W, H int }

// Place puts a page of w×h points onto the device:
//   - noscale: natural size, the page corner on the paper corner (what lies outside the printable area is cut off);
//   - fit: scaled up or down to the printable area, centred;
//   - shrink: like fit, but never enlarged.
func Place(w, h float64, d Device, scale string) Rect {
	nw, nh := w/72*float64(d.DPIX), h/72*float64(d.DPIY)
	f := min(float64(d.HorzRes)/nw, float64(d.VertRes)/nh)
	switch scale {
	case "fit":
	case "shrink":
		f = min(f, 1)
	default:
		return Rect{-d.OffX, -d.OffY, round(nw), round(nh)}
	}
	sw, sh := round(nw*f), round(nh*f)
	return Rect{(d.HorzRes - sw) / 2, (d.VertRes - sh) / 2, sw, sh}
}

// round rounds to the nearest pixel.
func round(f float64) int { return int(f + 0.5) }
