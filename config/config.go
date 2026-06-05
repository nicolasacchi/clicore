// Package config is the fleet-shared multi-project credential store. Save() is
// atomic (temp file in the same dir + os.Rename) so an interrupted or failed
// encode never corrupts a config.toml that already holds credentials — the bug
// present across the per-tool config layers that used a bare O_TRUNC open.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Project is one entry under [projects.<name>]. Tools may embed/extend this.
type Project struct {
	Name     string `toml:"name"`
	APIKey   string `toml:"api_key,omitempty"`
	WriteKey string `toml:"write_key,omitempty"` // optional read/write split
	BaseURL  string `toml:"base_url,omitempty"`
}

// File is the on-disk shape of config.toml.
type File struct {
	Default  string             `toml:"default_project"`
	Projects map[string]Project `toml:"projects"`
}

// Load reads and decodes the config at path. A missing file yields an empty
// File and no error (first-run friendly).
func Load(path string) (*File, error) {
	f := &File{Projects: map[string]Project{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(b, f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Projects == nil {
		f.Projects = map[string]Project{}
	}
	return f, nil
}

// Save writes the shared File shape to path atomically. Tools that keep their
// own config struct should call SaveTOML directly.
func Save(path string, f *File) error { return SaveTOML(path, f) }

// SaveTOML writes v as TOML to path atomically: encode to a temp file in the
// same dir (so os.Rename stays on one filesystem), fsync, then rename over the
// target. An interrupted or failed encode therefore never corrupts a
// config.toml that already holds credentials. Accepts any struct, so a tool can
// keep its own config shape and still get the atomic-write guarantee.
func SaveTOML(path string, v any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

// WriteFileAtomic writes data to path atomically: a temp file in the same dir
// (so os.Rename stays on one filesystem), fsync, then rename over the target.
// An interrupted or failed write therefore never corrupts a file that already
// holds data. Use it directly for non-TOML configs (e.g. a JSON credential
// store); SaveTOML is the TOML convenience built on top.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path) // atomic on POSIX
}

// FirstNonEmpty implements flag > env > file precedence resolution.
func FirstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// MaskSecret renders pk_ab***wxyz for safe `config list` display.
func MaskSecret(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:5] + "***" + s[len(s)-4:]
}
