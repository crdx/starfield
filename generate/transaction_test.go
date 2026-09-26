package generate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFailedBeginPreservesGeneratedConnectionState(t *testing.T) {
	response, err := Run(context.Background(), getRequest(t))
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	directory := t.TempDir()
	module := fmt.Sprintf("module fixture\n\ngo %s\n", goVersion(t))
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, fixtureFile), response.Files[0].Contents, 0o600); err != nil {
		t.Fatalf("write generated code: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "transaction_test.go"), []byte(failedBeginTest), 0o600); err != nil {
		t.Fatalf("write transaction test: %v", err)
	}

	command := exec.Command("go", "test", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("test generated transaction code: %v\n%s", err, output)
	}
}

const failedBeginTest = `package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

type failingBeginDriver struct{}
type failingBeginConnection struct{}

func (failingBeginDriver) Open(string) (driver.Conn, error) {
	return failingBeginConnection{}, nil
}

func (failingBeginConnection) Prepare(string) (driver.Stmt, error) {
	panic("unreachable")
}

func (failingBeginConnection) Close() error {
	return nil
}

func (failingBeginConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("begin failed")
}

func TestFailedBeginPreservesConnectionState(t *testing.T) {
	sql.Register("failing-begin", failingBeginDriver{})
	database, err := sql.Open("failing-begin", "")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	connection = database
	oldConnection = nil

	if err := BeginTransaction(); err == nil || err.Error() != "begin failed" {
		t.Fatalf("first BeginTransaction error = %v, want begin failed", err)
	}
	if oldConnection != nil {
		t.Error("failed BeginTransaction marked a transaction as open")
	}
	if connection != database {
		t.Error("failed BeginTransaction replaced the database connection")
	}
	if err := BeginTransaction(); err == nil || err.Error() != "begin failed" {
		t.Fatalf("second BeginTransaction error = %v, want begin failed", err)
	}
}
`
