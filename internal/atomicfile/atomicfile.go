// Package atomicfile writes files through a temporary sibling plus rename so
// readers never observe a partially written file.
package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
)

// Write replaces path with data. The temporary file is fsynced before the
// rename, and supported platforms also sync the parent directory afterwards,
// so both the contents and the replacement directory entry reach storage.
func Write(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".iptv-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncParent(filepath.Dir(path))
}

// WriteIfChanged skips the write entirely when path already holds data,
// sparing router flash from rewrites of identical EPG and playlist content.
// It reports whether the file was written.
func WriteIfChanged(path string, data []byte, mode os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := Write(path, data, mode); err != nil {
		return false, err
	}
	return true, nil
}
