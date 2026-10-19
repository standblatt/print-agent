package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestSetDisplayVersion(t *testing.T) {
	const key = `Software\StandblattPrintAgentTest`
	if err := setDisplayVersion(key, "0.2.1", `C:\Apps\x.exe`); err != nil {
		t.Fatalf("without an entry: %v", err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close(); registry.DeleteKey(registry.CURRENT_USER, key) })
	k.SetStringValue("UninstallString", `"C:\Users\x\AppData\Local\Programs\Standblatt Print Agent\uninstall.exe"`)
	k.SetStringValue("DisplayVersion", "0.2.0")
	version := func() string { v, _, _ := k.GetStringValue("DisplayVersion"); return v }

	if err := setDisplayVersion(key, "0.2.1", `D:\portable\standblatt-print-agent-app-windows-amd64.exe`); err != nil || version() != "0.2.0" {
		t.Fatalf("a portable exe changed the entry: %v %s", err, version())
	}
	if err := setDisplayVersion(key, "0.2.1", `c:\users\x\appdata\local\programs\standblatt print agent\standblatt-print-agent.exe`); err != nil || version() != "0.2.1" {
		t.Fatalf("the installed exe: %v %s", err, version())
	}
}
