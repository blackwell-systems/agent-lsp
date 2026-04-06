# Changelog

All notable changes to this project will be documented in this file.
The format is based on Keep a Changelog, Semantic Versioning.

## [Unreleased]

### Added
- Initial Go port of LSP-MCP
- All 24 tools from the TypeScript implementation
- 7-language CI verification (TypeScript, Python, Go, Rust, Java, C, PHP)
- Single binary distribution via `go install`
- Buffer-based LSP message framing (byte-accurate Content-Length handling)
- WaitForDiagnostics with 500ms stabilisation window
- Extension registry with compile-time factory registration
