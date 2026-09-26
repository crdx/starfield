package generate

import (
	"context"
	"testing"
)

func TestGenerateStringID(t *testing.T) {
	request := getRequest(t)
	for _, schema := range request.Catalog.Schemas {
		for _, table := range schema.Tables {
			if table.Rel.Name != "posts" {
				continue
			}
			for _, column := range table.Columns {
				if column.Name == "id" {
					column.Type.Name = "varchar"
					column.Unsigned = false
				}
			}
		}
	}

	response, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("generate string ID: %v", err)
	}
	assertCompiles(t, response.Files[0].Contents)
}
