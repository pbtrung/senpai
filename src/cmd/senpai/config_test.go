package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyConfigurationIsPrivateAndAtomic(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "original.scfg")
	target := filepath.Join(dir, "new", "nested", "senpai.scfg")
	content := []byte("password secret\n")
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyContents(target, source); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(content) {
		t.Fatalf("copied content: %q, %v", got, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("password file permissions: %o", info.Mode().Perm())
	}
	if err := copyContents(target, dir); err == nil {
		t.Fatal("copying a directory succeeded")
	}
	got, err = os.ReadFile(target)
	if err != nil || string(got) != string(content) {
		t.Fatal("failed copy replaced existing configuration")
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".senpai-config-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files remain: %v, %v", leftovers, err)
	}
}
