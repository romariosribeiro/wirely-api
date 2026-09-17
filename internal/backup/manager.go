package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
	_ "modernc.org/sqlite"
)

const (
	manifestName = "_wirely_backup.json"
	markerName   = "restore-pending.json"
)

var (
	ErrNotFound       = errors.New("backup not found")
	ErrInvalidArchive = errors.New("invalid Wirely backup")
)

type Info struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
	Reason    string    `json:"reason"`
	Version   int       `json:"version"`
}

type Status struct {
	Data          []Info        `json:"data"`
	Automatic     bool          `json:"automatic"`
	Interval      time.Duration `json:"-"`
	IntervalHours float64       `json:"intervalHours"`
	Retention     int           `json:"retention"`
	NextBackupAt  time.Time     `json:"nextBackupAt,omitempty"`
}

type manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Reason    string    `json:"reason"`
}

type restoreMarker struct {
	BackupID string `json:"backupId"`
}

type Manager struct {
	dataDirectory string
	backupDir     string
	retention     int
	interval      time.Duration

	mu           sync.Mutex
	nextBackupAt time.Time
	cancel       context.CancelFunc
}

func New(dataDirectory string, retention int, interval time.Duration) (*Manager, error) {
	if retention < 1 {
		retention = 7
	}
	backupDir := filepath.Join(dataDirectory, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}
	return &Manager{dataDirectory: dataDirectory, backupDir: backupDir, retention: retention, interval: interval}, nil
}

func (m *Manager) Start() {
	if m.interval <= 0 {
		return
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.nextBackupAt = time.Now().UTC().Add(m.interval)
	m.mu.Unlock()
	go m.loop(ctx)
}

func (m *Manager) Close() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *Manager) loop(ctx context.Context) {
	timer := time.NewTimer(m.interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_, _ = m.Create(ctx, "automatic")
			m.mu.Lock()
			m.nextBackupAt = time.Now().UTC().Add(m.interval)
			m.mu.Unlock()
			timer.Reset(m.interval)
		}
	}
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	items, err := m.List(ctx)
	if err != nil {
		return Status{}, err
	}
	m.mu.Lock()
	next := m.nextBackupAt
	m.mu.Unlock()
	return Status{Data: items, Automatic: m.interval > 0, Interval: m.interval,
		IntervalHours: m.interval.Hours(), Retention: m.retention, NextBackupAt: next}, nil
}

func (m *Manager) Create(ctx context.Context, reason string) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createLocked(ctx, reason, "")
}

func (m *Manager) createLocked(ctx context.Context, reason, preserveID string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}
	createdAt := time.Now().UTC()
	random, err := security.RandomToken(6)
	if err != nil {
		return Info{}, err
	}
	id := "wirely-" + createdAt.Format("20060102-150405") + "-" + random
	snapshot, err := os.MkdirTemp(m.backupDir, ".snapshot-")
	if err != nil {
		return Info{}, fmt.Errorf("create backup staging directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	if err := snapshotTree(ctx, m.dataDirectory, snapshot); err != nil {
		return Info{}, err
	}
	archivePath := filepath.Join(m.backupDir, id+".zip")
	if err := writeArchive(ctx, archivePath, snapshot, manifest{Version: 1, CreatedAt: createdAt, Reason: reason}); err != nil {
		_ = os.Remove(archivePath)
		return Info{}, err
	}
	stat, err := os.Stat(archivePath)
	if err != nil {
		return Info{}, err
	}
	if err := m.pruneLocked(preserveID); err != nil {
		return Info{}, err
	}
	return Info{ID: id, CreatedAt: createdAt, Size: stat.Size(), Reason: reason, Version: 1}, nil
}

func (m *Manager) List(ctx context.Context) ([]Info, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	items := make([]Info, 0)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "wirely-") || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := inspectArchive(filepath.Join(m.backupDir, entry.Name()))
		if err != nil {
			continue
		}
		items = append(items, info)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (m *Manager) Path(id string) (string, error) {
	if !validID(id) {
		return "", ErrNotFound
	}
	path := filepath.Join(m.backupDir, id+".zip")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) Delete(id string) error {
	path, err := m.Path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

func (m *Manager) PrepareRestore(ctx context.Context, id string) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.Path(id)
	if err != nil {
		return Info{}, err
	}
	selected, err := inspectArchive(path)
	if err != nil {
		return Info{}, err
	}
	if _, err := m.createLocked(ctx, "pre-restore", id); err != nil {
		return Info{}, fmt.Errorf("create pre-restore backup: %w", err)
	}
	marker, _ := json.Marshal(restoreMarker{BackupID: id})
	temporary := filepath.Join(m.backupDir, "."+markerName+".tmp")
	if err := os.WriteFile(temporary, marker, 0o600); err != nil {
		return Info{}, fmt.Errorf("stage restore marker: %w", err)
	}
	if err := os.Rename(temporary, filepath.Join(m.backupDir, markerName)); err != nil {
		return Info{}, fmt.Errorf("activate restore marker: %w", err)
	}
	return selected, nil
}

func (m *Manager) pruneLocked(preserveID string) error {
	items, err := m.List(context.Background())
	if err != nil {
		return err
	}
	toDelete := len(items) - m.retention
	for index := len(items) - 1; index >= 0 && toDelete > 0; index-- {
		if items[index].ID == preserveID {
			continue
		}
		if err := os.Remove(filepath.Join(m.backupDir, items[index].ID+".zip")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("prune backup: %w", err)
		}
		toDelete--
	}
	return nil
}

func validID(id string) bool {
	if !strings.HasPrefix(id, "wirely-") || len(id) > 100 {
		return false
	}
	for _, character := range id {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func snapshotTree(ctx context.Context, source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		if relative == "backups" || strings.HasPrefix(relative, "backups"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup refuses symbolic link %s", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if strings.HasSuffix(entry.Name(), "-wal") || strings.HasSuffix(entry.Name(), "-shm") {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".db") {
			return snapshotSQLite(path, target)
		}
		return copyFile(path, target, 0o600)
	})
}

func snapshotSQLite(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	dsn := (&url.URL{Scheme: "file", Path: source}).String() + "?_pragma=busy_timeout(10000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open SQLite backup source: %w", err)
	}
	defer db.Close()
	quoted := strings.ReplaceAll(destination, "'", "''")
	if _, err := db.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return fmt.Errorf("snapshot SQLite database %s: %w", filepath.Base(source), err)
	}
	return os.Chmod(destination, 0o600)
}

func copyFile(source, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func writeArchive(ctx context.Context, path, source string, metadata manifest) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	archive := zip.NewWriter(output)
	manifestWriter, err := archive.Create(manifestName)
	if err == nil {
		err = json.NewEncoder(manifestWriter).Encode(metadata)
	}
	if err == nil {
		err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || ctx.Err() != nil {
				if walkErr != nil {
					return walkErr
				}
				return ctx.Err()
			}
			if entry.IsDir() {
				return nil
			}
			relative, relErr := filepath.Rel(source, path)
			if relErr != nil {
				return relErr
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			header, headerErr := zip.FileInfoHeader(info)
			if headerErr != nil {
				return headerErr
			}
			header.Name = filepath.ToSlash(relative)
			header.Method = zip.Deflate
			writer, createErr := archive.CreateHeader(header)
			if createErr != nil {
				return createErr
			}
			input, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			_, copyErr := io.Copy(writer, input)
			closeErr := input.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
	}
	closeArchiveErr := archive.Close()
	closeFileErr := output.Close()
	if err != nil {
		return fmt.Errorf("write backup archive: %w", err)
	}
	if closeArchiveErr != nil {
		return closeArchiveErr
	}
	return closeFileErr
}

func inspectArchive(path string) (Info, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Info{}, ErrInvalidArchive
	}
	defer reader.Close()
	var metadata manifest
	found := false
	for _, file := range reader.File {
		if file.Name != manifestName {
			continue
		}
		input, err := file.Open()
		if err != nil {
			return Info{}, ErrInvalidArchive
		}
		err = json.NewDecoder(io.LimitReader(input, 64<<10)).Decode(&metadata)
		_ = input.Close()
		if err != nil || metadata.Version != 1 || metadata.CreatedAt.IsZero() {
			return Info{}, ErrInvalidArchive
		}
		found = true
		break
	}
	if !found {
		return Info{}, ErrInvalidArchive
	}
	stat, err := os.Stat(path)
	if err != nil {
		return Info{}, err
	}
	return Info{ID: strings.TrimSuffix(filepath.Base(path), ".zip"), CreatedAt: metadata.CreatedAt,
		Size: stat.Size(), Reason: metadata.Reason, Version: metadata.Version}, nil
}
