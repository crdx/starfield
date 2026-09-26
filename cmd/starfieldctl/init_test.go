package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitImportsGeneratedDatabasePackage(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("go.mod", []byte("module example.com/probe\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	doInit()

	contents, err := os.ReadFile(filepath.Join("db", "schema", "main.go"))
	if err != nil {
		t.Fatalf("read generated migration helper: %v", err)
	}
	if !strings.Contains(string(contents), `"example.com/probe/db"`) {
		t.Errorf("generated migration helper does not import database package:\n%s", contents)
	}
}
