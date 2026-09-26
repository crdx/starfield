package generate

import (
	"strings"
	"testing"

	"github.com/samber/mo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestQueryValueHelpers(t *testing.T) {
	empty := QueryValue{}
	if got := empty.Params(); got != "" {
		t.Errorf("empty Params() = %q", got)
	}
	if got := empty.Pairs(); got != nil {
		t.Errorf("empty Pairs() = %#v", got)
	}

	scalar := QueryValue{Name: "rows", Typ: "int64"}
	if got := scalar.Type(); got != "int64" {
		t.Errorf("scalar Type() = %q", got)
	}
	if got := scalar.Params(); got != "rows_" {
		t.Errorf("scalar Params() = %q, want rows_", got)
	}
	if got := scalar.VariableForField(Field{}); got != "rows" {
		t.Errorf("scalar VariableForField() = %q, want rows", got)
	}
	if got := scalar.ReturnName(); got != "rows" {
		t.Errorf("ReturnName() = %q, want rows", got)
	}

	fields := []Field{
		{Name: "One", Type: "int64", Column: testPluginColumn("one", "int", true, false)},
		{Name: "Two", Type: "int64", Column: testPluginColumn("two", "int", true, false)},
		{Name: "Three", Type: "int64", Column: testPluginColumn("three", "int", true, false)},
		{Name: "Four", Type: "int64", Column: testPluginColumn("four", "int", true, false)},
	}
	structured := QueryValue{Name: "params", EmitAsStruct: true, Struct: &Struct{Name: "Params", Fields: fields}}
	if got := structured.Type(); got != "Params" {
		t.Errorf("struct Type() = %q, want Params", got)
	}
	if got := structured.Params(); !strings.HasPrefix(got, "\nparams.One,\n") || !strings.HasSuffix(got, "params.Four,\n") {
		t.Errorf("multiline Params() = %q", got)
	}

	duplicated := QueryValue{Struct: &Struct{Fields: []Field{{Name: "Value"}, {Name: "Value"}}}}
	if got := len(duplicated.UniqueFields()); got != 1 {
		t.Errorf("UniqueFields() length = %d, want 1", got)
	}
}

func TestQueryValueTypePanicsWhenMissing(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Error("Type() did not panic")
		}
	}()
	_ = (QueryValue{Name: "missing"}).Type()
}

func TestQueryClassification(t *testing.T) {
	params := []*plugin.Parameter{
		{Column: &plugin.Column{}},
		{Column: &plugin.Column{Table: &plugin.Identifier{Schema: "public"}}},
		{Column: &plugin.Column{Table: &plugin.Identifier{}}},
	}
	updates, conditionals := classifyParams(params)
	if updates != 1 || conditionals != 2 {
		t.Errorf("classifyParams() = %d, %d, want 1, 2", updates, conditionals)
	}
	if shouldEmitAsStruct(params, -1) {
		t.Error("negative max params emitted a struct")
	}
	if !shouldEmitAsStruct(params, 2) {
		t.Error("params over the limit did not emit a struct")
	}
	if !shouldEmitAsStruct(params[:2], 3) {
		t.Error("mixed update and conditional params did not emit a struct")
	}
}

func TestMakeQueriesRejectsIncompatibleArguments(t *testing.T) {
	request := &plugin.GenerateRequest{
		Queries: []*plugin.Query{{
			Name: "Broken",
			Cmd:  ":exec",
			Params: []*plugin.Parameter{
				{Number: 1, Column: testPluginColumn("value", "text", true, true)},
				{Number: 2, Column: testPluginColumn("value", "int", true, true)},
			},
		}},
	}
	options := &Options{MaxParams: mo.Some(3)}
	if _, err := makeQueries(request, options, nil); err == nil {
		t.Fatal("makeQueries() accepted incompatible arguments")
	}
}

func TestMakeQueriesRejectsIncompatibleResultColumns(t *testing.T) {
	request := &plugin.GenerateRequest{
		Catalog: &plugin.Catalog{},
		Queries: []*plugin.Query{{
			Name: "Broken",
			Cmd:  ":many",
			Columns: []*plugin.Column{
				testPluginColumn("value", "text", true, true),
				testPluginColumn("value", "int", true, true),
			},
		}},
	}
	options := &Options{MaxParams: mo.Some(3)}
	if _, err := makeQueries(request, options, nil); err == nil {
		t.Fatal("makeQueries() accepted incompatible result columns")
	}
}

func TestQueryDispatchDefaults(t *testing.T) {
	query := Query{Command: ":unknown"}
	if got := getMethod(query); got != "Exec" {
		t.Errorf("getMethod() = %q, want Exec", got)
	}
	if got := getReturnValue(query); got != "" {
		t.Errorf("getReturnValue() = %q, want empty", got)
	}
}
