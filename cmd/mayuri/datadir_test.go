package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDataDir(t *testing.T) {
	dir := t.TempDir()
	if err := checkDataDir(filepath.Join(dir, "mayuri.db")); err != nil {
		t.Fatalf("writable dir: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("check left files behind: %v", entries)
	}

	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	err := checkDataDir(filepath.Join(ro, "mayuri.db"))
	want := fmt.Sprintf("database directory %s is not writable by uid %d", ro, os.Getuid())
	if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "chown -R") {
		t.Fatalf("read-only dir: %v", err)
	}

	if err := checkDataDir(filepath.Join(dir, "missing", "mayuri.db")); err == nil {
		t.Fatal("missing directory should fail")
	}
}
