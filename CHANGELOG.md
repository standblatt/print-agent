# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Protocol client and run loop of the print agent, moved from the server repository; printing goes through a backend per operating system (by @HB9HIL)
- CUPS backend for Linux and macOS: printers and options from `lpstat` and `lpoptions`, printing with `lp`, all taken from `PATH`; the variables `STANDBLATT_LP`, `STANDBLATT_LPSTAT` and `STANDBLATT_LPOPTIONS` are gone (by @HB9HIL)
- Headless agent `standblatt-print-agent`, configured by `STANDBLATT_URL` and `STANDBLATT_AGENT_TOKEN` (by @HB9HIL)
- CI with gofmt, vet, staticcheck, govulncheck, doc comment check and tests; releases carry the Linux binaries (amd64, arm64) and `SHA256SUMS` (by @HB9HIL)
- Contract test in CI: the agent runs against a pinned Standblatt server image and must report its printers; a wrong token must be refused (by @HB9HIL)
- Makefile with `prepare`, `test` (Go tests and the contract test in Docker) and `clean` (by @HB9HIL)
- Windows backend (`internal/winspool`) for the desktop app: printers, trays and papers from the spooler, PDFs rendered with PDFium in WebAssembly (no CGO, no helper program) and printed through GDI; options `bin`, `paper`, `duplex`, `scale` (default `noscale`) and `color`; Windows CI job that prints through Microsoft Print to PDF (by @HB9HIL)
- Desktop app *Standblatt Print Agent* (`app/`, Wails 3, own Go module) for Windows: scaffold with a Svelte frontend in English, German, French and Italian, the exe builds without CGO; CI builds it and checks it on Windows (by @HB9HIL)
- The app's core: it pairs ranges by code, keeps their tokens in a settings file only the user can read, runs an agent per range on the Windows spooler (CUPS on macOS), prints a local test page and logs to a file it rotates at 1 MB (by @HB9HIL)
- The app's window: per range whether it prints (connected, printers missing, no connection, removed in Standblatt) with its last jobs, connecting a range by code with a button that opens Standblatt, the printers of the computer with a test page, and under *Advanced* start with the computer, the address of Standblatt and removing a range (by @HB9HIL)
- The app lives in the system tray (green when every range prints, red otherwise, with *Open* and *Quit*), closing the window only hides it, a second start shows the window of the first, and on its first start it turns on starting with the computer, in the tray (by @HB9HIL)
- Make targets for the builds: `app` (the desktop app for the system of this computer), `app-windows`, `app-mac` (a bundle signed ad hoc), `app-linux` and `app-headless` (the headless agent) (by @HB9HIL)
- `make dev` runs the desktop app on the developer's computer through `wails3 dev`, without turning on autostart (by @HB9HIL)
- The agent keeps a status for the desktop app: last contact, last error, the server's answer to hello and the last 20 jobs (by @HB9HIL)
- Pairing by code in the agent package (`Agent.Pair`, `Agent.WaitPaired`) for the desktop app: the agent asks the server for a code, waits until a member confirms it on `/connect` and gets its token once (by @HB9HIL)

### Changed
- The contract test runs against Standblatt 0.21.0, prints a test page and pairs through `Agent.Pair` and `Agent.WaitPaired` (by @HB9HIL)
- The agent reports its version and operating system with its printers and logs the club, the range and the printers of the printing setup it does not find; it needs Standblatt 0.21.0 (by @HB9HIL)
- The CUPS backend reports only the tray, paper size and media type options (`InputSlot`, `PageSize`, `MediaType`); other `lp` options remain free text in the printing setup (by @HB9HIL)

### Fixed
- A CUPS printer without options no longer makes the server refuse the whole printer report (by @HB9HIL)
