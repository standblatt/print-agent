// Package cups is the print backend for Linux and macOS: it lists printers with lpstat and lpoptions and prints
// with lp, all taken from PATH.
package cups

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"

	"github.com/standblatt/print-agent/internal/agent"
)

// reported are the options the agent reports per printer: the ones an admin picks per document. Others, such as
// fit-to-page, still work as free-text options of a print setting.
var reported = []string{"InputSlot", "PageSize", "MediaType"}

// CUPS prints through the CUPS command line tools.
type CUPS struct {
	Log *slog.Logger
}

// ParseOptions reads the output of `lpoptions -p <printer> -l`, e.g. "InputSlot/Media Source: Auto *Tray1 Tray2";
// never nil, the server refuses null arrays.
func ParseOptions(out string) []agent.Option {
	opts := []agent.Option{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		head, values, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name, label, _ := strings.Cut(head, "/")
		o := agent.Option{Name: strings.TrimSpace(name), Label: strings.TrimSpace(label), Values: []string{}}
		for _, v := range strings.Fields(values) {
			if strings.HasPrefix(v, "*") {
				v = v[1:]
				o.Default = v
			}
			o.Values = append(o.Values, v)
		}
		if o.Name != "" {
			opts = append(opts, o)
		}
	}
	return opts
}

// Printers lists the local CUPS printers with their tray, paper size and media type options.
func (c CUPS) Printers(ctx context.Context) ([]agent.Printer, error) {
	out, err := exec.CommandContext(ctx, "lpstat", "-e").Output()
	if err != nil {
		return nil, fmt.Errorf("lpstat: %w", err)
	}
	printers := []agent.Printer{}
	for _, name := range strings.Fields(string(out)) {
		opts, err := exec.CommandContext(ctx, "lpoptions", "-p", name, "-l").Output()
		if err != nil {
			c.Log.Warn("lpoptions", "printer", name, "err", err)
		}
		all := ParseOptions(string(opts))
		printers = append(printers, agent.Printer{Name: name, Options: slices.DeleteFunc(all, func(o agent.Option) bool {
			return !slices.Contains(reported, o.Name)
		})})
	}
	return printers, nil
}

// Print sends a job to CUPS with lp; an option with an empty value is passed as a bare `-o name`.
func (c CUPS) Print(ctx context.Context, j *agent.Job) error {
	f, err := os.CreateTemp("", "standblatt-*.pdf")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(j.PDF); err != nil {
		f.Close()
		return err
	}
	f.Close()
	args := []string{"-d", j.Printer, "-t", j.Title}
	keys := make([]string, 0, len(j.Options))
	for k := range j.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := j.Options[k]; v == "" {
			args = append(args, "-o", k)
		} else {
			args = append(args, "-o", k+"="+v)
		}
	}
	args = append(args, f.Name())
	out, err := exec.CommandContext(ctx, "lp", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("lp: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
