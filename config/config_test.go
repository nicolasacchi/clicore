package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSave_Mode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml")
	f := &File{Default: "prod", Projects: map[string]Project{"prod": {Name: "prod", APIKey: "pk_secret"}}}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("want 0600, got %o", st.Mode().Perm())
	}
}

// BUG-FIX #1: a successful Save leaves no stray temp file and round-trips; an
// O_TRUNC impl would instead truncate the live file before encoding.
func TestConfig_AtomicSaveLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	in := &File{Default: "a", Projects: map[string]Project{"a": {Name: "a", APIKey: "k"}}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("stray temp file left: %s", e.Name())
		}
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Default != "a" || out.Projects["a"].APIKey != "k" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestLoad_MissingFileIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Projects) != 0 || f.Default != "" {
		t.Fatalf("missing file should yield empty config, got %+v", f)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "env", "file"); got != "env" {
		t.Errorf("got %q, want env", got)
	}
	if got := FirstNonEmpty("flag", "env"); got != "flag" {
		t.Errorf("got %q, want flag", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("pk_abcdefghwxyz"); got != "pk_ab***wxyz" {
		t.Errorf("got %q", got)
	}
	if got := MaskSecret("short"); got != "*****" {
		t.Errorf("short secret should be all stars, got %q", got)
	}
}
