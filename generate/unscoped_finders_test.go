package generate

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateSingularUnscopedFinders(t *testing.T) {
	response, err := Run(context.Background(), getRequest(t))
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	code := string(response.Files[0].Contents)
	if !strings.Contains(code, "func FindUserByEmailUnscoped(value string) (*User, bool)") {
		t.Error("soft-deletable model does not have a singular unscoped finder")
	}
	if strings.Contains(code, "func FindPostByTitleUnscoped") {
		t.Error("model without deleted_at has an irrelevant singular unscoped finder")
	}
	assertCompiles(t, response.Files[0].Contents)
}
