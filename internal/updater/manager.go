package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAPIURL = "https://api.github.com/repos/romariosribeiro/wirely-api/releases/latest"
	maxArchive    = 200 << 20
)

var (
	ErrNoUpdate          = errors.New("Wirely is already up to date")
	ErrUpdateUnavailable = errors.New("update package is unavailable for this system")
)

type Status struct {
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   string    `json:"latestVersion,omitempty"`
	UpdateAvailable bool      `json:"updateAvailable"`
	CanApply        bool      `json:"canApply"`
	Title           string    `json:"title,omitempty"`
	Notes           string    `json:"notes,omitempty"`
	PublishedAt     time.Time `json:"publishedAt,omitempty"`
	ReleaseURL      string    `json:"releaseUrl,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
	Message         string    `json:"message,omitempty"`
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	Tag         string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	HTMLURL     string         `json:"html_url"`
	PublishedAt time.Time      `json:"published_at"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []releaseAsset `json:"assets"`
}

type Manager struct {
	currentVersion  string
	dataDirectory   string
	apiURL          string
	client          *http.Client
	selfUpdateReady func() bool

	mu        sync.Mutex
	cached    Status
	cachedRel release
	cachedAt  time.Time
}

func New(currentVersion, dataDirectory string) *Manager {
	return &Manager{
		currentVersion: strings.TrimPrefix(strings.TrimSpace(currentVersion), "v"),
		dataDirectory:  dataDirectory,
		apiURL:         defaultAPIURL,
		client:         &http.Client{Timeout: 45 * time.Second},
		selfUpdateReady: func() bool {
			_, err := os.Stat("/etc/systemd/system/wirely-update.path")
			return err == nil
		},
	}
}

func (m *Manager) Check(ctx context.Context, refresh bool) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !refresh && !m.cachedAt.IsZero() && time.Since(m.cachedAt) < 5*time.Minute {
		return m.cached, nil
	}
	rel, found, err := m.fetchRelease(ctx)
	if err != nil {
		return Status{}, err
	}
	now := time.Now().UTC()
	status := Status{CurrentVersion: m.currentVersion, CheckedAt: now}
	if !found {
		status.Message = "Nenhuma versão estável foi publicada ainda."
		m.cached, m.cachedRel, m.cachedAt = status, release{}, now
		return status, nil
	}
	status.LatestVersion = strings.TrimPrefix(rel.Tag, "v")
	status.Title = strings.TrimSpace(rel.Name)
	if status.Title == "" {
		status.Title = rel.Tag
	}
	status.Notes = strings.TrimSpace(rel.Body)
	status.PublishedAt = rel.PublishedAt
	status.ReleaseURL = rel.HTMLURL
	status.UpdateAvailable = compareVersions(status.LatestVersion, status.CurrentVersion) > 0
	_, _, available := releaseAssets(rel)
	status.CanApply = status.UpdateAvailable && available && m.selfUpdateReady()
	if status.UpdateAvailable && !available {
		status.Message = "A release não possui um pacote compatível com este servidor."
	} else if status.UpdateAvailable && !status.CanApply {
		status.Message = "Execute o instalador atual para habilitar a atualização automática com systemd."
	}
	m.cached, m.cachedRel, m.cachedAt = status, rel, now
	return status, nil
}

func (m *Manager) Apply(ctx context.Context) (Status, error) {
	status, err := m.Check(ctx, true)
	if err != nil {
		return Status{}, err
	}
	if !status.UpdateAvailable {
		return status, ErrNoUpdate
	}
	m.mu.Lock()
	rel := m.cachedRel
	m.mu.Unlock()
	archiveAsset, checksumAsset, ok := releaseAssets(rel)
	if !ok {
		return status, ErrUpdateUnavailable
	}
	archive, err := m.download(ctx, archiveAsset.URL, maxArchive)
	if err != nil {
		return status, fmt.Errorf("download update: %w", err)
	}
	checksumBody, err := m.download(ctx, checksumAsset.URL, 4096)
	if err != nil {
		return status, fmt.Errorf("download checksum: %w", err)
	}
	want := strings.Fields(string(checksumBody))
	if len(want) == 0 || len(want[0]) != sha256.Size*2 {
		return status, errors.New("release checksum is invalid")
	}
	if _, err := hex.DecodeString(want[0]); err != nil {
		return status, errors.New("release checksum is invalid")
	}
	sum := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), want[0]) {
		return status, errors.New("release checksum does not match")
	}
	binary, err := extractBinary(archive)
	if err != nil {
		return status, err
	}
	if err := m.stageBinary(binary); err != nil {
		return status, err
	}
	return status, nil
}

func (m *Manager) fetchRelease(ctx context.Context) (release, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.apiURL, nil)
	if err != nil {
		return release{}, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wirely-api/"+m.currentVersion)
	response, err := m.client.Do(req)
	if err != nil {
		return release{}, false, fmt.Errorf("check GitHub release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return release{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return release{}, false, fmt.Errorf("GitHub release check returned HTTP %d", response.StatusCode)
	}
	var rel release
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&rel); err != nil {
		return release{}, false, fmt.Errorf("decode GitHub release: %w", err)
	}
	if rel.Draft || rel.Prerelease || strings.TrimSpace(rel.Tag) == "" {
		return release{}, false, nil
	}
	return rel, true, nil
}

func (m *Manager) download(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return nil, errors.New("release asset URL is not trusted")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wirely-api/"+m.currentVersion)
	response, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, errors.New("release asset is too large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("release asset is too large")
	}
	return body, nil
}

func releaseAssets(rel release) (releaseAsset, releaseAsset, bool) {
	archiveName := fmt.Sprintf("wirely-%s-linux-%s.tar.gz", rel.Tag, runtime.GOARCH)
	checksumName := archiveName + ".sha256"
	var archive, checksum releaseAsset
	for _, asset := range rel.Assets {
		switch asset.Name {
		case archiveName:
			archive = asset
		case checksumName:
			checksum = asset
		}
	}
	return archive, checksum, archive.URL != "" && checksum.URL != ""
}

func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.New("release archive is invalid")
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("release archive is invalid")
		}
		if filepath.Base(header.Name) != "wirely" || header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Size < 1 || header.Size > maxArchive {
			return nil, errors.New("release binary has an invalid size")
		}
		binary, err := io.ReadAll(io.LimitReader(reader, maxArchive+1))
		if err != nil || int64(len(binary)) != header.Size {
			return nil, errors.New("release binary is invalid")
		}
		if len(binary) < 4 || !bytes.Equal(binary[:4], []byte{0x7f, 'E', 'L', 'F'}) {
			return nil, errors.New("release binary is not a Linux executable")
		}
		return binary, nil
	}
	return nil, errors.New("release archive does not contain the Wirely binary")
}

func (m *Manager) stageBinary(binary []byte) error {
	directory := filepath.Join(m.dataDirectory, "updates")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create update staging directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".wirely-update-*")
	if err != nil {
		return fmt.Errorf("stage update: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o755); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(binary); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	ready := filepath.Join(directory, "wirely.ready")
	if err := os.Rename(temporaryPath, ready); err != nil {
		return fmt.Errorf("stage verified update: %w", err)
	}
	return nil
}

func (m *Manager) Activate() error {
	directory := filepath.Join(m.dataDirectory, "updates")
	ready := filepath.Join(directory, "wirely.ready")
	pending := filepath.Join(directory, "wirely.pending")
	if err := os.Rename(ready, pending); err != nil {
		return fmt.Errorf("activate staged update: %w", err)
	}
	return nil
}

func compareVersions(left, right string) int {
	parse := func(value string) [3]int {
		value = strings.SplitN(strings.TrimPrefix(value, "v"), "-", 2)[0]
		parts := strings.Split(value, ".")
		var result [3]int
		for index := 0; index < len(parts) && index < len(result); index++ {
			result[index], _ = strconv.Atoi(parts[index])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := range a {
		if a[index] > b[index] {
			return 1
		}
		if a[index] < b[index] {
			return -1
		}
	}
	return 0
}
