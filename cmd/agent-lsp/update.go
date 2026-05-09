package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// githubRelease represents the relevant fields from the GitHub Releases API.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a single release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

const releasesURL = "https://api.github.com/repos/blackwell-systems/agent-lsp/releases/latest"

// assetName returns the expected asset filename for the given OS and architecture.
func assetName(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("agent-lsp_%s_%s%s", goos, goarch, ext)
}

// stripVersionPrefix removes a leading "v" from a version string.
func stripVersionPrefix(v string) string {
	return strings.TrimPrefix(v, "v")
}

// fetchLatestRelease fetches the latest release metadata from GitHub.
func fetchLatestRelease(client *http.Client) (*githubRelease, error) {
	req, err := http.NewRequest("GET", releasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agent-lsp/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release JSON: %w", err)
	}
	return &release, nil
}

// findAsset locates the correct asset for the current platform.
func findAsset(assets []githubAsset, goos, goarch string) (*githubAsset, error) {
	want := assetName(goos, goarch)
	for i := range assets {
		if assets[i].Name == want {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("no asset found for %s/%s (expected %s)", goos, goarch, want)
}

// downloadToFile downloads a URL to a temporary file in dir and returns the path.
func downloadToFile(client *http.Client, url, dir string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(dir, "agent-lsp-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("writing download: %w", err)
	}
	tmp.Close()
	return tmp.Name(), nil
}

// extractBinaryFromTarGz reads a .tar.gz and extracts the first regular file
// (the binary) to a temporary file in dir. Returns the path to the extracted file.
func extractBinaryFromTarGz(tarPath, dir string) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("no binary found in tarball")
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}
		// Skip directories; take the first regular file.
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		tmp, err := os.CreateTemp(dir, "agent-lsp-extracted-*")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(tmp, tr); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", fmt.Errorf("extracting binary: %w", err)
		}
		tmp.Close()
		return tmp.Name(), nil
	}
}

func runUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "Check for updates without downloading")
	force := fs.Bool("force", false, "Update even if already on the latest version")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	client := &http.Client{}
	release, err := fetchLatestRelease(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	currentVersion := stripVersionPrefix(Version)
	latestVersion := stripVersionPrefix(release.TagName)

	if *checkOnly {
		fmt.Printf("Current version: %s\n", currentVersion)
		fmt.Printf("Latest version:  %s\n", latestVersion)
		if currentVersion == latestVersion {
			fmt.Println("You are up to date.")
		} else {
			fmt.Println("An update is available.")
		}
		return
	}

	if currentVersion == latestVersion && !*force {
		fmt.Printf("Already up to date (v%s).\n", currentVersion)
		return
	}

	// Locate the correct asset for this platform.
	asset, err := findAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Resolve the path of the running binary.
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		os.Exit(1)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving symlinks: %v\n", err)
		os.Exit(1)
	}
	execDir := filepath.Dir(execPath)

	fmt.Printf("Downloading %s...\n", asset.Name)
	dlPath, err := downloadToFile(client, asset.BrowserDownloadURL, execDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(dlPath) // clean up if something goes wrong

	var newBinaryPath string
	if strings.HasSuffix(asset.Name, ".tar.gz") {
		newBinaryPath, err = extractBinaryFromTarGz(dlPath, execDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting archive: %v\n", err)
			os.Exit(1)
		}
		os.Remove(dlPath) // remove tarball; keep extracted binary
	} else {
		// Raw binary or zip; for now treat as raw binary.
		newBinaryPath = dlPath
	}

	// Set executable permissions.
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting permissions: %v\n", err)
		os.Remove(newBinaryPath)
		os.Exit(1)
	}

	// Atomic replace: rename new binary over current binary.
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error replacing binary: %v\n", err)
		os.Remove(newBinaryPath)
		os.Exit(1)
	}

	fmt.Printf("Updated agent-lsp: v%s -> v%s\n", currentVersion, latestVersion)
}
