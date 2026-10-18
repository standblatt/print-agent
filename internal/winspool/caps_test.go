package winspool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/standblatt/print-agent/internal/agent"
)

// office is an office printer as the HP Universal PS driver reports it: repeated bin names, duplex, no colour.
var office = Caps{
	Bins:    []Entry{{15, "Automatisch auswählen"}, {259, "Fach 1"}, {260, "Fach 2"}, {1298, "Karton"}, {1299, "Karton"}},
	Papers:  []Entry{{9, "A4"}, {11, "A5"}},
	Duplex:  true,
	Default: Settings{Bin: 15, Paper: 9, Duplex: 1, Color: 1},
}

func TestOptions(t *testing.T) {
	got := Options(office)
	want := []agent.Option{
		{Name: "bin", Label: "Tray", Values: []string{"Automatisch auswählen", "Fach 1", "Fach 2", "Karton"}, Default: "Automatisch auswählen"},
		{Name: "paper", Label: "Paper", Values: []string{"A4", "A5"}, Default: "A4"},
		{Name: "duplex", Label: "Two-sided", Values: []string{"simplex", "duplexlong", "duplexshort"}, Default: "simplex"},
		{Name: "scale", Label: "Scaling", Values: []string{"noscale", "shrink", "fit"}, Default: "noscale"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("office:\n got %+v\nwant %+v", got, want)
	}

	pdf := Options(Caps{Papers: []Entry{{9, "A4"}}, Color: true, Default: Settings{Paper: 9, Duplex: 3, Color: 1}})
	want = []agent.Option{
		{Name: "paper", Label: "Paper", Values: []string{"A4"}, Default: "A4"},
		{Name: "scale", Label: "Scaling", Values: []string{"noscale", "shrink", "fit"}, Default: "noscale"},
		{Name: "color", Label: "Colour", Values: []string{"color", "monochrome"}, Default: "monochrome"},
	}
	if !reflect.DeepEqual(pdf, want) {
		t.Fatalf("no bins, no duplex, colour:\n got %+v\nwant %+v", pdf, want)
	}

	short := Options(Caps{Duplex: true, Default: Settings{Duplex: 3}})
	if short[0].Name != "duplex" || short[0].Default != "duplexshort" {
		t.Fatalf("duplex default: %+v", short)
	}
}

func TestTranslate(t *testing.T) {
	colour := office
	colour.Color = true
	for _, c := range []struct {
		name string
		caps Caps
		opts map[string]string
		want Settings
		err  string
	}{
		{"nothing keeps the defaults", office, nil, Settings{Scale: "noscale"}, ""},
		{"every option", colour, map[string]string{"bin": "Fach 2", "paper": "A5", "duplex": "duplexshort", "scale": "fit", "color": "monochrome"},
			Settings{Bin: 260, Paper: 11, Duplex: 3, Color: 1, Scale: "fit"}, ""},
		{"long edge and colour", colour, map[string]string{"duplex": "duplexlong", "color": "color"}, Settings{Duplex: 2, Color: 2, Scale: "noscale"}, ""},
		{"a repeated name takes the first ID", office, map[string]string{"bin": "Karton"}, Settings{Bin: 1298, Scale: "noscale"}, ""},
		{"unknown tray", office, map[string]string{"bin": "Fach 3"}, Settings{}, `no tray "Fach 3"`},
		{"unknown paper", office, map[string]string{"paper": "A3"}, Settings{}, `no paper "A3"`},
		{"duplex without a duplex unit", Caps{}, map[string]string{"duplex": "duplexlong"}, Settings{}, "cannot print"},
		{"colour on a mono printer", office, map[string]string{"color": "color"}, Settings{}, "cannot print in"},
		{"unknown scaling", office, map[string]string{"scale": "stretch"}, Settings{}, "unknown scaling"},
		{"CUPS option", office, map[string]string{"InputSlot": "Tray2"}, Settings{}, `unknown option "InputSlot"`},
	} {
		got, err := Translate(c.opts, c.caps)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: got error %v, want %q", c.name, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v", c.name, got, err, c.want)
		}
	}
}

func TestPlace(t *testing.T) {
	// A4 at 600 dpi with 100 px margins on every side.
	a4 := Device{DPIX: 600, DPIY: 600, OffX: 100, OffY: 100, HorzRes: 4760, VertRes: 6816}
	const a4w, a4h, a5w, a5h = 595.28, 841.89, 419.53, 595.28
	for _, c := range []struct {
		name  string
		w, h  float64
		scale string
		want  Rect
	}{
		{"A4 noscale on the paper corner", a4w, a4h, "noscale", Rect{-100, -100, 4961, 7016}},
		{"A4 fit into the printable area", a4w, a4h, "fit", Rect{0, 42, 4760, 6732}},
		{"A4 shrink equals fit when too big", a4w, a4h, "shrink", Rect{0, 42, 4760, 6732}},
		{"A5 fit enlarges", a5w, a5h, "fit", Rect{0, 31, 4760, 6754}},
		{"A5 shrink keeps the size, centred", a5w, a5h, "shrink", Rect{632, 927, 3496, 4961}},
		{"A5 noscale on the paper corner", a5w, a5h, "noscale", Rect{-100, -100, 3496, 4961}},
	} {
		if got := Place(c.w, c.h, a4, c.scale); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
