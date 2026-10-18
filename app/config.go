package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultServer is where new ranges are paired unless the user sets another server.
const defaultServer = "https://app.standblatt.ch"

// Config is the app's settings file; it holds the agent tokens, so only the user can read it.
type Config struct {
	Server string  `json:"server"`
	Ranges []Range `json:"ranges"`
}

// Range is a range this computer prints for, with the server it was paired with.
type Range struct {
	ID     string `json:"id"`
	Server string `json:"server"`
	Club   string `json:"club"`
	Range  string `json:"range"`
	Token  string `json:"token"`
}

// loadConfig reads the config at path; a missing file gives the defaults and created true.
func loadConfig(path string) (c Config, created bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Server: defaultServer}, true, nil
	}
	if err != nil {
		return c, false, err
	}
	return c, false, json.Unmarshal(b, &c)
}

// saveConfig writes the config to path, readable by the user only, through a temporary file so a crash never
// leaves half a file.
func saveConfig(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".print-agent-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
