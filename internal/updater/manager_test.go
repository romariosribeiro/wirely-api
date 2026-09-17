package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func response(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), ContentLength: int64(len(body))}
}

func releaseArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var result bytes.Buffer
	gz := gzip.NewWriter(&result)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "wirely", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestApplyVerifiedReleaseAndActivatePendingBinary(t *testing.T) {
	newBinary := []byte("\x7fELFnew-wirely")
	archive := releaseArchive(t, newBinary)
	sum := sha256.Sum256(archive)
	archiveName := fmt.Sprintf("wirely-v1.2.0-linux-%s.tar.gz", runtime.GOARCH)
	releaseJSON := fmt.Sprintf(`{"tag_name":"v1.2.0","name":"Wirely 1.2.0","body":"Mudanças","html_url":"https://github.com/romariosribeiro/wirely-api/releases/tag/v1.2.0","published_at":"2026-09-17T12:00:00Z","assets":[{"name":%q,"browser_download_url":"https://github.com/download/archive"},{"name":%q,"browser_download_url":"https://github.com/download/checksum"}]}`, archiveName, archiveName+".sha256")

	directory := t.TempDir()
	manager := New("1.1.0", directory)
	manager.selfUpdateReady = func() bool { return true }
	manager.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/romariosribeiro/wirely-api/releases/latest":
			return response(http.StatusOK, []byte(releaseJSON)), nil
		case "/download/archive":
			return response(http.StatusOK, archive), nil
		case "/download/checksum":
			return response(http.StatusOK, []byte(hex.EncodeToString(sum[:])+"  "+archiveName+"\n")), nil
		default:
			return response(http.StatusNotFound, nil), nil
		}
	})}

	status, err := manager.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "1.2.0" {
		t.Fatalf("unexpected status: %#v", status)
	}
	ready, _ := os.ReadFile(filepath.Join(directory, "updates", "wirely.ready"))
	if !bytes.Equal(ready, newBinary) {
		t.Fatalf("unexpected staged binary=%q", ready)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	pending, _ := os.ReadFile(filepath.Join(directory, "updates", "wirely.pending"))
	if !bytes.Equal(pending, newBinary) {
		t.Fatalf("unexpected pending binary=%q", pending)
	}
}

func TestVersionComparisonAndInvalidArchive(t *testing.T) {
	if compareVersions("1.2.0", "1.1.9") <= 0 || compareVersions("1.2.0", "1.2.0") != 0 || compareVersions("0.9.0", "1.0.0") >= 0 {
		t.Fatal("semantic version comparison failed")
	}
	archive := releaseArchive(t, []byte("not-elf"))
	if _, err := extractBinary(archive); err == nil || !strings.Contains(err.Error(), "Linux executable") {
		t.Fatalf("expected invalid executable error, got %v", err)
	}
}
