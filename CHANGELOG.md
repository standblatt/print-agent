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
- Makefile with `build`, `prepare`, `test` (Go tests and the contract test in Docker) and `clean` (by @HB9HIL)
- Windows backend (`internal/winspool`) for the desktop app: printers, trays and papers from the spooler, PDFs rendered with PDFium in WebAssembly (no CGO, no helper program) and printed through GDI; options `bin`, `paper`, `duplex`, `scale` (default `noscale`) and `color`; Windows CI job that prints through Microsoft Print to PDF (by @HB9HIL)
