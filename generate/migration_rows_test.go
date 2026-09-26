package generate

import (
	"context"
	"strings"
	"testing"
)

func TestGeneratedMigrationChecksRowsError(t *testing.T) {
	response, err := Run(context.Background(), getRequest(t))
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	code := string(response.Files[0].Contents)
	migrationStart := strings.Index(code, "func migrate(")
	transactionStart := strings.Index(code, "func BeginTransaction(")
	if migrationStart < 0 || transactionStart < 0 {
		t.Fatal("generated migration function not found")
	}
	migrationCode := code[migrationStart:transactionStart]

	rowsClose := strings.Index(migrationCode, "rows.Close()")
	rowsError := strings.Index(migrationCode, "rows.Err()")
	migrationLoop := strings.Index(migrationCode, "for _, migration := range config.Migrations")
	if rowsClose < 0 || rowsError < rowsClose || migrationLoop < rowsError {
		t.Errorf("migration row error is not checked before migrations run:\n%s", migrationCode)
	}
}
