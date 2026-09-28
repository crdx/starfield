package generate

import (
	"context"
	"strings"
	"testing"

	"github.com/samber/mo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestRunRejectsInvalidOptions(t *testing.T) {
	request := getRequest(t)
	request.PluginOptions = []byte("{")

	if _, err := Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "unmarshalling plugin options") {
		t.Fatalf("Run() error = %v, want invalid options error", err)
	}
}

func TestRunRejectsInvalidQuery(t *testing.T) {
	request := getRequest(t)
	request.Queries = []*plugin.Query{{
		Name: "Broken",
		Cmd:  ":exec",
		Params: []*plugin.Parameter{
			{Number: 1, Column: testPluginColumn("value", "text", true, true)},
			{Number: 2, Column: testPluginColumn("value", "int", true, true)},
		},
	}}

	if _, err := Run(context.Background(), request); err == nil {
		t.Fatal("Run() accepted an invalid query")
	}
}

func TestGenerateReportsTemplateError(t *testing.T) {
	request := &plugin.GenerateRequest{
		Settings:    &plugin.Settings{Engine: "mysql"},
		SqlcVersion: "test",
	}
	options := &Options{Package: "db", MaxParams: mo.Some(3)}
	structs := []Struct{{Name: "Empty", Table: &plugin.Identifier{Name: "empty"}}}

	if _, err := generate(request, options, structs, nil); err == nil {
		t.Fatal("generate() accepted an empty model")
	}
}

func TestGenerateReportsFormattingError(t *testing.T) {
	request := &plugin.GenerateRequest{
		Settings:    &plugin.Settings{Engine: "mysql"},
		SqlcVersion: "test",
	}
	options := &Options{Package: "db", MaxParams: mo.Some(3)}
	queries := []Query{{
		Command:      ":exec",
		ConstantName: "invalidQuery",
		MethodName:   "123Invalid",
		SourceName:   "queries",
		SQL:          "select 1",
	}}

	if _, err := generate(request, options, nil, queries); err == nil || !strings.Contains(err.Error(), "unable to format queries") {
		t.Fatalf("generate() error = %v, want formatting error", err)
	}
}

func TestRunRejectsEmbedWithoutModel(t *testing.T) {
	request := getRequest(t)
	request.Queries = []*plugin.Query{{
		Name: "ListMissing",
		Cmd:  ":many",
		Columns: []*plugin.Column{
			{Name: "missing", EmbedTable: &plugin.Identifier{Name: "missing"}},
		},
	}}

	if _, err := Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "ListMissing: sqlc.embed(missing): no model for table missing") {
		t.Fatalf("Run() error = %v, want missing model error", err)
	}
}

func TestRunRejectsUnsupportedCommands(t *testing.T) {
	for _, command := range []string{":copyfrom", ":batchexec", ":batchmany", ":batchone"} {
		request := getRequest(t)
		request.Queries = []*plugin.Query{{Name: "Bulk", Cmd: command}}

		if _, err := Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "Bulk: "+command+" is not supported") {
			t.Errorf("Run() error = %v, want %s refused", err, command)
		}
	}
}
