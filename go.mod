module github.com/blackwell-systems/agent-lsp

go 1.25.0

// Dependencies populated by `go mod tidy` after Wave 1 agents run.
// Agent E (Wave 2) runs `go mod tidy` to resolve github.com/modelcontextprotocol/go-sdk.

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/google/jsonschema-go v0.4.2
	github.com/modelcontextprotocol/go-sdk v1.4.1
	modernc.org/sqlite v1.50.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	modernc.org/libc v1.72.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
