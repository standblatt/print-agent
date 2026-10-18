package main

import (
	"os"
	"path/filepath"
)

// maxLog is the size above which the log is moved to .1 at start.
const maxLog = 1 << 20

// openLog opens the log file at path for appending; a log over maxLog becomes path.1 first, replacing the older one.
func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLog {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
