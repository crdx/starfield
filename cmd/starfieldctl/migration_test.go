package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGetUsageDescribesCommands(t *testing.T) {
	usage := getUsage()
	for _, text := range []string{"init", "make-migration", "version", "--unix", "--target"} {
		if !strings.Contains(usage, text) {
			t.Errorf("getUsage() does not contain %q", text)
		}
	}
}

func TestGetMigrationID(t *testing.T) {
	t.Run("timestamp", func(t *testing.T) {
		identifier := getMigrationID("CreateFoos", false)
		if !regexp.MustCompile(`^\d{14}_create_foos$`).MatchString(identifier) {
			t.Fatalf("getMigrationID() = %q", identifier)
		}

		timestamp := strings.TrimSuffix(identifier, "_create_foos")
		if _, err := time.Parse("20060102150405", timestamp); err != nil {
			t.Errorf("parse migration timestamp: %v", err)
		}
	})

	t.Run("unix", func(t *testing.T) {
		before := time.Now().UTC().Unix()
		identifier := getMigrationID("CreateFoos", true)
		after := time.Now().UTC().Unix()

		timestamp := strings.TrimSuffix(identifier, "_create_foos")
		got, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			t.Fatalf("parse Unix migration timestamp: %v", err)
		}
		if got < before || got > after {
			t.Errorf("migration timestamp = %d, want between %d and %d", got, before, after)
		}
	})
}

func TestMakeMigration(t *testing.T) {
	t.Chdir(t.TempDir())

	config := "sql:\n  - name: primary\n    schema: db/primary\n  - name: archive\n    schema: db/archive\n"
	if err := os.WriteFile(sqlcFile, []byte(config), 0o600); err != nil {
		t.Fatalf("write sqlc config: %v", err)
	}

	output := captureStdout(t, func() {
		makeMigration("CreateAuditEvents", false, "archive")
	})
	if !strings.Contains(output, "created") {
		t.Errorf("makeMigration() output = %q, want created message", output)
	}

	matches, err := filepath.Glob(filepath.Join("db", "archive", "*_create_audit_events.sql"))
	if err != nil {
		t.Fatalf("glob migration: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("created migrations = %v, want one", matches)
	}

	info, err := os.Stat(matches[0])
	if err != nil {
		t.Fatalf("stat migration: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Errorf("migration permissions = %o, want 600", permissions)
	}
}

func TestOutputMigrationMessage(t *testing.T) {
	output := captureStdout(t, func() {
		outputMigrationMessage(true, "migration.sql", "created")
	})
	if !strings.Contains(output, "migration.sql") || !strings.Contains(output, "[created]") {
		t.Errorf("outputMigrationMessage() = %q", output)
	}
}

func captureStdout(t *testing.T, action func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create output pipe: %v", err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
	}()

	action()
	if err := writer.Close(); err != nil {
		t.Fatalf("close output writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close output reader: %v", err)
	}

	return string(output)
}
