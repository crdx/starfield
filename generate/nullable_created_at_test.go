package generate

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateNullableCreatedAt(t *testing.T) {
	request := getRequest(t)
	for _, schema := range request.Catalog.Schemas {
		for _, table := range schema.Tables {
			if table.Rel.Name != "posts" {
				continue
			}
			for _, column := range table.Columns {
				if column.Name == "created_at" {
					column.NotNull = false
				}
			}
		}
	}

	response, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("generate nullable created_at: %v", err)
	}

	code := response.Files[0].Contents
	if !strings.Contains(string(code), "!value.CreatedAt.Valid || value.CreatedAt.V.IsZero()") {
		t.Error("nullable created_at does not use its validity and underlying value")
	}
	assertCompiles(t, code)
}
