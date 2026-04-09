module github.com/blackwell-systems/agent-lsp

go 1.25.0

// Dependencies populated by `go mod tidy` after Wave 1 agents run.
// Agent E (Wave 2) runs `go mod tidy` to resolve github.com/modelcontextprotocol/go-sdk.

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/modelcontextprotocol/go-sdk v1.4.1
)

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)
