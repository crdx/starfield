package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSchemaDir(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "sqlc.yml")
	contents := "sql:\n  - name: primary\n    schema: db/primary\n  - name: archive\n    schema: db/archive\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	schemaDirectory, err := resolveSchemaDir(configPath, "archive")
	if err != nil {
		t.Fatalf("resolve schema directory: %v", err)
	}
	if schemaDirectory != "db/archive" {
		t.Errorf("resolveSchemaDir() = %q, want db/archive", schemaDirectory)
	}

	if _, err := resolveSchemaDir(filepath.Join(directory, "missing.yml"), ""); err == nil {
		t.Error("resolveSchemaDir() accepted a missing config")
	}
}

func TestReadConfig(t *testing.T) {
	directory := t.TempDir()

	t.Run("valid", func(t *testing.T) {
		configPath := filepath.Join(directory, "valid.yml")
		contents := "sql:\n  - name: app\n    schema: db/schema\n"
		if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		config, err := readConfig(configPath)
		if err != nil {
			t.Fatalf("readConfig() error: %v", err)
		}
		if len(config.SQL) != 1 || config.SQL[0].Name != "app" || config.SQL[0].Schema != "db/schema" {
			t.Errorf("readConfig() = %#v", config)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		configPath := filepath.Join(directory, "invalid.yml")
		if err := os.WriteFile(configPath, []byte("sql: ["), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if _, err := readConfig(configPath); err == nil {
			t.Error("readConfig() accepted invalid YAML")
		}
	})
}

func TestReadModulePath(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
		wantErr  bool
	}{
		{name: "plain", contents: "module example.com/plain\n", want: "example.com/plain"},
		{name: "quoted", contents: "module \"example.com/quoted\"\n", want: "example.com/quoted"},
		{name: "backtick quoted", contents: "module `example.com/backtick`\n", want: "example.com/backtick"},
		{name: "missing directive", contents: "go 1.25.0\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modulePath := filepath.Join(t.TempDir(), "go.mod")
			if err := os.WriteFile(modulePath, []byte(test.contents), 0o600); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}

			got, err := readModulePath(modulePath)
			if test.wantErr {
				if err == nil {
					t.Error("readModulePath() succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("readModulePath() error: %v", err)
			}
			if got != test.want {
				t.Errorf("readModulePath() = %q, want %q", got, test.want)
			}
		})
	}

	if _, err := readModulePath(filepath.Join(t.TempDir(), "missing.mod")); err == nil {
		t.Error("readModulePath() accepted a missing file")
	}
}

func TestGetSchemaDir(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		target      string
		want        string
		wantErrText string
	}{
		{name: "no blocks", config: &Config{}, wantErrText: "no sql blocks"},
		{
			name:   "single block",
			config: &Config{SQL: []Entry{{Schema: "db/schema"}}},
			want:   "db/schema",
		},
		{
			name: "named target",
			config: &Config{SQL: []Entry{
				{Name: "primary", Schema: "db/primary"},
				{Name: "archive", Schema: "db/archive"},
			}},
			target: "archive",
			want:   "db/archive",
		},
		{
			name: "multiple blocks without target",
			config: &Config{SQL: []Entry{
				{Name: "primary", Schema: "db/primary"},
				{Name: "archive", Schema: "db/archive"},
			}},
			wantErrText: "available: primary, archive",
		},
		{
			name: "unknown target",
			config: &Config{SQL: []Entry{
				{Name: "primary", Schema: "db/primary"},
				{Name: "archive", Schema: "db/archive"},
			}},
			target:      "missing",
			wantErrText: "no sql block named missing (available: primary, archive)",
		},
		{
			name: "unnamed blocks",
			config: &Config{SQL: []Entry{
				{Schema: "db/primary"},
				{Schema: "db/archive"},
			}},
			target:      "missing",
			wantErrText: "no sql blocks are named",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := getSchemaDir(test.config, test.target)
			if test.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
					t.Fatalf("getSchemaDir() error = %v, want containing %q", err, test.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("getSchemaDir() error: %v", err)
			}
			if got != test.want {
				t.Errorf("getSchemaDir() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "present")
	if exists(path) {
		t.Error("exists() reports a missing path")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if !exists(path) {
		t.Error("exists() does not report an existing path")
	}
}

func TestSnakeCase(t *testing.T) {
	tests := map[string]string{
		"createFoos": "create_foos",
		"CreateFoos": "create_foos",
		"foos":       "foos",
		"HTTPServer": "h_t_t_p_server",
	}

	for input, want := range tests {
		if got := snakeCase(input); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", input, got, want)
		}
	}
}
