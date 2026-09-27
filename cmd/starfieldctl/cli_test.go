package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const cliTestArguments = "STARFIELDCTL_TEST_ARGUMENTS"

func TestCLI(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/cli\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	output, err := runCLI(t, directory, "version")
	if err != nil {
		t.Fatalf("run version: %v\n%s", err, output)
	}
	if strings.TrimSpace(output) != Version {
		t.Errorf("version output = %q, want %q", output, Version)
	}

	output, err = runCLI(t, directory, "init")
	if err != nil {
		t.Fatalf("run init: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(directory, "sqlc.yml")); err != nil {
		t.Errorf("init did not create sqlc.yml: %v", err)
	}

	output, err = runCLI(t, directory, "make-migration", "CreateWidgets", "--unix")
	if err != nil {
		t.Fatalf("run make-migration: %v\n%s", err, output)
	}
	matches, err := filepath.Glob(filepath.Join(directory, "db", "schema", "*_create_widgets.sql"))
	if err != nil {
		t.Fatalf("glob migration: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("created migrations = %v, want one", matches)
	}

	output, err = runCLI(t, directory, "unknown")
	if err == nil {
		t.Fatal("unknown command succeeded")
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 2 {
		t.Fatalf("unknown command error = %v, want exit status 2", err)
	}
	if !strings.Contains(output, "Usage:") {
		t.Errorf("unknown command output = %q, want usage", output)
	}
}

func TestCLIHelper(t *testing.T) {
	t.Helper()

	arguments := os.Getenv(cliTestArguments)
	if arguments == "" {
		return
	}

	os.Args = append([]string{"starfieldctl"}, strings.Split(arguments, "\n")...)
	main()
}

func runCLI(t *testing.T, directory string, arguments ...string) (string, error) {
	t.Helper()

	command := exec.Command(os.Args[0], "-test.run=^TestCLIHelper$") //nolint:gosec // The current test binary is a trusted executable.
	command.Dir = directory
	command.Env = append(os.Environ(), cliTestArguments+"="+strings.Join(arguments, "\n"))
	output, err := command.CombinedOutput()

	return string(output), err
}
