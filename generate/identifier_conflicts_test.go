package generate

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateEscapesQueryIdentifierConflicts(t *testing.T) {
	request := getRequest(t)
	for _, query := range request.Queries {
		if query.Name != "CountUsers" {
			continue
		}
		query.Name = "Type"
		query.Columns[0].Name = "err"
	}

	response, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("generate conflicting identifiers: %v", err)
	}

	code := string(response.Files[0].Contents)
	if !strings.Contains(code, "const type_ =") {
		t.Error("query constant does not escape Go keyword")
	}
	if !strings.Contains(code, "var err_ int64") {
		t.Error("return value does not escape generated local name")
	}
	assertCompiles(t, response.Files[0].Contents)
}
