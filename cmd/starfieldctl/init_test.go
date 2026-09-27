package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crdx.org/starfield/scaffold"
)

func TestInitWritesScaffoldAndPreservesExistingFiles(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)

	if err := os.WriteFile("go.mod", []byte("module example.com/probe\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	doInit()

	assertFileContains(t, sqlcFile, "name: "+filepath.Base(directory))
	assertFileContains(t, filepath.Join("db", "schema", "main.go"), `"example.com/probe/db"`)
	assertFileEquals(t, filepath.Join("db", "schema", "00000000000000_init.sql"), scaffold.MigrationSQL)
	assertFileEquals(t, filepath.Join("db", "queries", "foos.sql"), scaffold.QuerySQL)

	paths := []string{
		sqlcFile,
		filepath.Join("db", "schema", "main.go"),
		filepath.Join("db", "schema", "00000000000000_init.sql"),
		filepath.Join("db", "queries", "foos.sql"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("preserved"), 0o600); err != nil {
			t.Fatalf("replace %s: %v", path, err)
		}
	}

	doInit()

	for _, path := range paths {
		assertFileEquals(t, path, []byte("preserved"))
	}
}

func TestInitReportsDirectoryCreationFailure(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("db", nil, 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	doInit()

	if !exists(sqlcFile) {
		t.Error("doInit() did not create sqlc.yml before the directory failure")
	}
	if info, err := os.Stat("db"); err != nil || !info.Mode().IsRegular() {
		t.Errorf("blocking file changed: info=%v error=%v", info, err)
	}
}

func assertFileContains(t *testing.T, path string, substring string) {
	t.Helper()

	contents, err := os.ReadFile(path) //nolint:gosec // Test paths are controlled by the caller.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(contents), substring) {
		t.Errorf("%s does not contain %q:\n%s", path, substring, contents)
	}
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()

	contents, err := os.ReadFile(path) //nolint:gosec // Test paths are controlled by the caller.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(contents) != string(want) {
		t.Errorf("%s = %q, want %q", path, contents, want)
	}
}
