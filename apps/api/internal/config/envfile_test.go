package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSecretFile_TrimsNewline(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s")
	if err := os.WriteFile(p, []byte("mysecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSecretFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "mysecret" {
		t.Fatalf("got %q", got)
	}
}

func TestGetEnvOrFile_PrefersEnv(t *testing.T) {
	t.Setenv("MY_KEY", "from-env")
	t.Setenv("MY_KEY_FILE", "") // unset file override
	dir := t.TempDir()
	fp := filepath.Join(dir, "f")
	if err := os.WriteFile(fp, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := getEnvOrFile("MY_KEY", "MY_KEY_FILE", fp)
	if got != "from-env" {
		t.Fatalf("got %q want from-env", got)
	}
}

func TestGetEnvOrFile_UsesFileEnv(t *testing.T) {
	t.Setenv("MY_KEY", "")
	dir := t.TempDir()
	fp := filepath.Join(dir, "f")
	if err := os.WriteFile(fp, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MY_KEY_FILE", fp)
	got := getEnvOrFile("MY_KEY", "MY_KEY_FILE", "")
	if got != "from-file" {
		t.Fatalf("got %q", got)
	}
}

func TestGetEnvOrFile_DefaultPath(t *testing.T) {
	t.Setenv("MY_KEY", "")
	t.Setenv("MY_KEY_FILE", "")
	dir := t.TempDir()
	def := filepath.Join(dir, "default")
	if err := os.WriteFile(def, []byte("default-path\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := getEnvOrFile("MY_KEY", "MY_KEY_FILE", def)
	if got != "default-path" {
		t.Fatalf("got %q", got)
	}
}
