package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// uninstallKey is where the installer registers the app for Settings → Apps.
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\StandblattPrintAgent`

// markInstalledVersion shows version in Settings → Apps, which the installer wrote but an in-app update does not.
func markInstalledVersion(version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return setDisplayVersion(uninstallKey, version, exe)
}

// setDisplayVersion sets the DisplayVersion of the uninstall entry key when exe is in the directory the installer
// put the app in; without an entry, or for a portable exe elsewhere, it does nothing.
func setDisplayVersion(key, version, exe string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	uninstaller, _, err := k.GetStringValue("UninstallString")
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Dir(strings.Trim(uninstaller, `"`)), filepath.Dir(exe)) {
		return nil
	}
	return k.SetStringValue("DisplayVersion", version)
}
