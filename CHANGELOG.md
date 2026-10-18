# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Protocol client and run loop of the print agent, moved from the server repository; printing goes through a backend per operating system, and the agent reports its version and operating system with its printers (by @HB9HIL)
