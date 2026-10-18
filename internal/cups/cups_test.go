package cups

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/standblatt/print-agent/internal/agent"
)

// fakeCUPS puts the fake lp, lpstat and lpoptions from testdata first in PATH and returns the lp log.
func fakeCUPS(t *testing.T) string {
	dir, err := filepath.Abs("../../testdata/fakecups")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(t.TempDir(), "lp.log")
	t.Setenv("FAKECUPS_LOG", log)
	return log
}

func TestParseOptions(t *testing.T) {
	out := "PageSize/Media Size: A4 *A5 A6 Custom.WIDTHxHEIGHT\nInputSlot/Media Source: *Auto Tray1 Tray2\nnot an option line\nDuplex/2-Sided Printing: *None DuplexNoTumble\n"
	got := ParseOptions(out)
	want := []agent.Option{
		{Name: "PageSize", Label: "Media Size", Values: []string{"A4", "A5", "A6", "Custom.WIDTHxHEIGHT"}, Default: "A5"},
		{Name: "InputSlot", Label: "Media Source", Values: []string{"Auto", "Tray1", "Tray2"}, Default: "Auto"},
		{Name: "Duplex", Label: "2-Sided Printing", Values: []string{"None", "DuplexNoTumble"}, Default: "None"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestPrinters(t *testing.T) {
	fakeCUPS(t)
	got, err := CUPS{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}.Printers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []agent.Printer{
		{Name: "Office_A5", Options: []agent.Option{{Name: "InputSlot", Label: "Media Source", Values: []string{"Auto", "Tray1", "Tray2"}, Default: "Auto"}}},
		{Name: "Brother_QL", Options: []agent.Option{{Name: "PageSize", Label: "Media Size", Values: []string{"29x90mm"}, Default: "29x90mm"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestPrint(t *testing.T) {
	log := fakeCUPS(t)
	j := &agent.Job{Title: "Lane sheet Anna", Printer: "Office_A5", PDF: []byte("%PDF-1.7 rest"),
		Options: map[string]string{"PageSize": "A5", "InputSlot": "Tray2", "fit-to-page": ""}}
	if err := (CUPS{}).Print(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	lines := strings.Split(string(b), "\n")
	if !strings.HasPrefix(lines[0], "-d Office_A5 -t Lane sheet Anna -o InputSlot=Tray2 -o PageSize=A5 -o fit-to-page ") || lines[1] != "%PDF-" {
		t.Fatalf("lp log: %q", b)
	}
}

func TestPrintError(t *testing.T) {
	fakeCUPS(t)
	t.Setenv("FAKECUPS_LOG", "/nonexistent/dir/lp.log")
	err := (CUPS{}).Print(context.Background(), &agent.Job{Printer: "Office_A5", PDF: []byte("%PDF")})
	if err == nil || !strings.HasPrefix(err.Error(), "lp: ") {
		t.Fatalf("got %v", err)
	}
}
