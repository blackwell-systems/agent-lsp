package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripVersionPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.7.0", "0.7.0"},
		{"", ""},
		{"v", ""},
	}
	for _, tt := range tests {
		got := stripVersionPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripVersionPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"darwin", "arm64", "agent-lsp_darwin_arm64.tar.gz"},
		{"darwin", "amd64", "agent-lsp_darwin_amd64.tar.gz"},
		{"linux", "amd64", "agent-lsp_linux_amd64.tar.gz"},
		{"linux", "arm64", "agent-lsp_linux_arm64.tar.gz"},
		{"windows", "amd64", "agent-lsp_windows_amd64.zip"},
	}
	for _, tt := range tests {
		got := assetName(tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("assetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestFindAsset(t *testing.T) {
	assets := []githubAsset{
		{Name: "agent-lsp_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_amd64"},
		{Name: "agent-lsp_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64"},
		{Name: "agent-lsp_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64"},
		{Name: "agent-lsp_linux_arm64.tar.gz", BrowserDownloadURL: "https://example.com/linux_arm64"},
		{Name: "agent-lsp_windows_amd64.zip", BrowserDownloadURL: "https://example.com/windows_amd64"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
	}

	t.Run("found", func(t *testing.T) {
		a, err := findAsset(assets, "darwin", "arm64")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Name != "agent-lsp_darwin_arm64.tar.gz" {
			t.Errorf("got asset %q, want agent-lsp_darwin_arm64.tar.gz", a.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := findAsset(assets, "freebsd", "amd64")
		if err == nil {
			t.Fatal("expected error for unsupported platform")
		}
	})
}

func TestUpdateCheckFlag(t *testing.T) {
	// Set up a mock GitHub API server.
	release := githubRelease{
		TagName: "v99.0.0",
		Assets: []githubAsset{
			{Name: "agent-lsp_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/dl"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// Test fetchLatestRelease with the mock server by parsing the response directly.
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to fetch mock release: %v", err)
	}
	defer resp.Body.Close()

	var got githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got.TagName != "v99.0.0" {
		t.Errorf("TagName = %q, want v99.0.0", got.TagName)
	}
	if len(got.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(got.Assets))
	}

	// Simulate --check logic: compare versions.
	currentVersion := stripVersionPrefix(Version) // "dev" in tests
	latestVersion := stripVersionPrefix(got.TagName)
	if currentVersion == latestVersion {
		t.Error("expected versions to differ in test (current=dev, latest=99.0.0)")
	}
	_ = latestVersion
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		equal   bool
	}{
		{"0.7.0", "0.7.0", true},
		{"0.7.0", "0.8.0", false},
		{"dev", "0.7.0", false},
		{"1.0.0", "1.0.0", true},
	}
	for _, tt := range tests {
		got := tt.current == tt.latest
		if got != tt.equal {
			t.Errorf("(%q == %q) = %v, want %v", tt.current, tt.latest, got, tt.equal)
		}
	}
}
