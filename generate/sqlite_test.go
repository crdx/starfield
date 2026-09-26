package generate

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateSQLiteWithoutMySQLMigrationLock(t *testing.T) {
	request := getRequest(t)
	request.Settings.Engine = "sqlite"

	response, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("generate SQLite code: %v", err)
	}

	code := response.Files[0].Contents
	if strings.Contains(string(code), "get_lock") {
		t.Error("SQLite output contains MySQL migration lock")
	}
	if strings.Contains(string(code), "dsn.DBName") {
		t.Error("SQLite output dereferences the optional DSN during migration")
	}

	assertCompiles(t, code)
}
