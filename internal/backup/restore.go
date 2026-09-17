package backup

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ApplyPending applies a staged restore before any Wirely database is opened.
// The backups directory is preserved and is never sourced from the archive.
func ApplyPending(dataDirectory string) (bool, error) {
	backupDir := filepath.Join(dataDirectory, "backups")
	markerPath := filepath.Join(backupDir, markerName)
	markerBody, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read restore marker: %w", err)
	}
	var marker restoreMarker
	if json.Unmarshal(markerBody, &marker) != nil || !validID(marker.BackupID) {
		return false, ErrInvalidArchive
	}
	archivePath := filepath.Join(backupDir, marker.BackupID+".zip")
	if _, err := inspectArchive(archivePath); err != nil {
		return false, err
	}
	parent := filepath.Dir(dataDirectory)
	stage, err := os.MkdirTemp(parent, ".wirely-restore-stage-")
	if err != nil {
		return false, fmt.Errorf("create restore staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(archivePath, stage); err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(stage, "wirely.db")); err != nil {
		return false, fmt.Errorf("%w: wirely.db is missing", ErrInvalidArchive)
	}

	rollback, err := os.MkdirTemp(parent, ".wirely-restore-rollback-")
	if err != nil {
		return false, fmt.Errorf("create restore rollback directory: %w", err)
	}
	defer os.RemoveAll(rollback)
	currentEntries, err := os.ReadDir(dataDirectory)
	if err != nil {
		return false, fmt.Errorf("read current data directory: %w", err)
	}
	for _, entry := range currentEntries {
		if entry.Name() == "backups" {
			continue
		}
		if err := os.Rename(filepath.Join(dataDirectory, entry.Name()), filepath.Join(rollback, entry.Name())); err != nil {
			_ = rollbackData(dataDirectory, rollback)
			return false, fmt.Errorf("stage current data for restore: %w", err)
		}
	}
	stagedEntries, err := os.ReadDir(stage)
	if err == nil {
		for _, entry := range stagedEntries {
			if err = os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(dataDirectory, entry.Name())); err != nil {
				break
			}
		}
	}
	if err != nil {
		_ = removeDataExceptBackups(dataDirectory)
		_ = rollbackData(dataDirectory, rollback)
		return false, fmt.Errorf("activate restored data: %w", err)
	}
	if err := os.Remove(markerPath); err != nil {
		return false, fmt.Errorf("clear restore marker: %w", err)
	}
	return true, nil
}

func extractArchive(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return ErrInvalidArchive
	}
	defer reader.Close()
	var extracted int64
	for _, file := range reader.File {
		if file.Name == manifestName {
			continue
		}
		name := filepath.Clean(filepath.FromSlash(file.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) ||
			name == "backups" || strings.HasPrefix(name, "backups"+string(filepath.Separator)) {
			return ErrInvalidArchive
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(destination, name), 0o700); err != nil {
				return err
			}
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 || file.UncompressedSize64 > 8<<30 {
			return ErrInvalidArchive
		}
		extracted += int64(file.UncompressedSize64)
		if extracted > 20<<30 {
			return ErrInvalidArchive
		}
		target := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return ErrInvalidArchive
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if closeInputErr != nil {
			return closeInputErr
		}
	}
	return nil
}

func removeDataExceptBackups(dataDirectory string) error {
	entries, err := os.ReadDir(dataDirectory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "backups" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataDirectory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func rollbackData(dataDirectory, rollback string) error {
	entries, err := os.ReadDir(rollback)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(rollback, entry.Name()), filepath.Join(dataDirectory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
