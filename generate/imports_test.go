package generate

import (
	"reflect"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestQueryImportsAllValueShapes(t *testing.T) {
	timeField := Field{Name: "CreatedAt", Type: "time.Time", Column: testPluginColumn("created_at", "datetime", true, false)}
	nullTimeField := Field{Name: "UpdatedAt", Type: "sql.Null[time.Time]", Column: testPluginColumn("updated_at", "datetime", false, false)}
	queries := []Query{
		{Command: ":one", ReturnValue: QueryValue{Name: "createdAt", Typ: "time.Time"}},
		{Command: ":many", ReturnValue: QueryValue{Name: "item", EmitAsStruct: true, Struct: &Struct{Name: "Row", Fields: []Field{timeField}}}},
		{Argument: QueryValue{Name: "params", EmitAsStruct: true, Struct: &Struct{Name: "Params", Fields: []Field{nullTimeField}}}},
		{Argument: QueryValue{Name: "params", Struct: &Struct{Name: "Params", Fields: []Field{timeField}}}},
	}

	imports := (&Importer{Queries: queries}).queryImports()
	if !reflect.DeepEqual(imports.Std, []string{"database/sql", "time"}) {
		t.Errorf("standard imports = %#v", imports.Std)
	}
}

func TestQueryImportsStringsForSlices(t *testing.T) {
	column := testPluginColumn("ids", "int", true, false)
	column.IsSqlcSlice = true
	queries := []Query{{Argument: QueryValue{Name: "ids", Typ: "[]int64", Column: column}}}

	imports := (&Importer{Queries: queries}).queryImports()
	if !reflect.DeepEqual(imports.Std, []string{"strings"}) {
		t.Errorf("standard imports = %#v", imports.Std)
	}
}

func TestSortAndMergeImports(t *testing.T) {
	sorted := sortImports(
		map[string]bool{"time": true, "database/sql": true},
		map[string]bool{"example.com/z": true, "example.com/a": true},
	)
	if !reflect.DeepEqual(sorted.Std, []string{"database/sql", "time"}) {
		t.Errorf("sorted standard imports = %#v", sorted.Std)
	}
	if !reflect.DeepEqual(sorted.Dep, []string{"example.com/a", "example.com/z"}) {
		t.Errorf("sorted dependency imports = %#v", sorted.Dep)
	}

	single := mergeImports(FileImports{Std: []string{"time"}, Dep: []string{"example.com/a"}})
	if !reflect.DeepEqual(single, [][]string{{"time"}, {"example.com/a"}}) {
		t.Errorf("single merged imports = %#v", single)
	}

	merged := mergeImports(
		FileImports{Std: []string{"time"}, Dep: []string{"example.com/a"}},
		FileImports{Std: []string{"time", "strings"}, Dep: []string{"example.com/a", "example.com/b"}},
	)
	want := [][]string{{"time", "strings"}, {"example.com/a", "example.com/b"}}
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged imports = %#v, want %#v", merged, want)
	}
}

func TestFixNamingConflicts(t *testing.T) {
	queries := []Query{{Argument: QueryValue{Name: "params", Struct: &Struct{Name: "Params"}}}}
	fixed := fixNamingConflicts([]string{"example.com/params"}, queries)
	if got := fixed[0].Argument.Name; got != "argParams" {
		t.Errorf("argument name = %q, want argParams", got)
	}

	unchanged := fixNamingConflicts([]string{"example.com/other"}, queries)
	if got := unchanged[0].Argument.Name; got != "params" {
		t.Errorf("argument name = %q, want params", got)
	}
}

func TestMainImporterUsesModelTypes(t *testing.T) {
	importer := &Importer{Structs: []Struct{{Fields: []Field{{Type: "sql.Null[time.Time]", Column: &plugin.Column{}}}}}}
	imports := importer.mainImports()
	if !reflect.DeepEqual(imports.Std, []string{
		"bytes", "context", "database/sql", "errors", "fmt", "log", "net/url", "path/filepath", "reflect", "regexp", "runtime", "strings", "time",
	}) {
		t.Errorf("main standard imports = %#v", imports.Std)
	}
}
