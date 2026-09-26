package generate

import (
	"context"
	"testing"
)

func TestGenerateWithoutPluginOptions(t *testing.T) {
	request := getRequest(t)
	request.PluginOptions = nil

	response, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("generate without plugin options: %v", err)
	}
	if len(response.Files) != 1 {
		t.Fatalf("generated %d files, want 1", len(response.Files))
	}

	assertCompiles(t, response.Files[0].Contents)
}
