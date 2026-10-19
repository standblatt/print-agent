//go:build !windows

package main

// markInstalledVersion does nothing: only the Windows installer registers the app with a version.
func markInstalledVersion(string) error { return nil }
