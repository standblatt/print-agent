package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Standblatt", "print-agent.json")
	c, created, err := loadConfig(path)
	if err != nil || !created || c.Server != defaultServer || len(c.Ranges) != 0 {
		t.Fatalf("defaults: %+v %v %v", c, created, err)
	}
	c.Ranges = append(c.Ranges, Range{ID: "a", Server: "https://x", Club: "SG Testdorf", Range: "25 m", Token: "tok"})
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, c); err != nil {
		t.Fatal("saving over an existing file:", err)
	}
	got, created, err := loadConfig(path)
	if err != nil || created || !reflect.DeepEqual(got, c) {
		t.Fatalf("round trip: %+v %v %v", got, created, err)
	}
	fi, _ := os.Stat(path)
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".print-agent-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
	os.WriteFile(path, []byte("{"), 0o600)
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("a broken file must be an error, not silently the defaults")
	}
}

func TestOpenLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Standblatt", "print-agent.log")
	f, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(make([]byte, maxLog+1))
	f.Close()
	f, err = openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("new\n")
	f.Close()
	if b, _ := os.ReadFile(path); string(b) != "new\n" {
		t.Fatalf("log after moving: %q", b)
	}
	if fi, err := os.Stat(path + ".1"); err != nil || fi.Size() != maxLog+1 {
		t.Fatalf("old log: %v %v", fi, err)
	}
}
