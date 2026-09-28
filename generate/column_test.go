package generate

import (
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestGetType(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		notNull  bool
		unsigned bool
		length   int32
		want     string
	}{
		{name: "string", typeName: "varchar", notNull: true, want: "string"},
		{name: "nullable string", typeName: "text", want: "sql.Null[string]"},
		{name: "boolean tinyint", typeName: "tinyint", notNull: true, length: 1, want: "bool"},
		{name: "nullable boolean tinyint", typeName: "tinyint", length: 1, want: "sql.Null[bool]"},
		{name: "tinyint", typeName: "tinyint", notNull: true, length: -1, want: "int64"},
		{name: "unsigned tinyint", typeName: "tinyint", notNull: true, unsigned: true, length: -1, want: "uint64"},
		{name: "wide tinyint", typeName: "tinyint", notNull: true, length: 4, want: "int64"},
		{name: "nullable tinyint", typeName: "tinyint", length: -1, want: "sql.Null[int64]"},
		{name: "signed integer", typeName: "int", notNull: true, want: "int64"},
		{name: "unsigned integer", typeName: "bigint", notNull: true, unsigned: true, want: "uint64"},
		{name: "nullable signed integer", typeName: "smallint", want: "sql.Null[int64]"},
		{name: "nullable unsigned integer", typeName: "mediumint", unsigned: true, want: "sql.Null[uint64]"},
		{name: "bytes", typeName: "blob", notNull: true, want: "[]byte"},
		{name: "nullable bytes", typeName: "binary", want: "sql.Null[string]"},
		{name: "float", typeName: "double", notNull: true, want: "float64"},
		{name: "nullable float", typeName: "real", want: "sql.Null[float64]"},
		{name: "decimal", typeName: "decimal", notNull: true, want: "string"},
		{name: "nullable decimal", typeName: "fixed", want: "sql.Null[string]"},
		{name: "time", typeName: "datetime", notNull: true, want: "time.Time"},
		{name: "nullable time", typeName: "timestamp", want: "sql.Null[time.Time]"},
		{name: "boolean", typeName: "boolean", notNull: true, want: "bool"},
		{name: "nullable boolean", typeName: "bool", want: "sql.Null[bool]"},
		{name: "enum", typeName: "enum", notNull: true, want: "string"},
		{name: "nullable enum", typeName: "enum", want: "sql.Null[string]"},
		{name: "json", typeName: "json", notNull: true, want: "json.RawMessage"},
		{name: "nullable json", typeName: "json", want: "sql.Null[json.RawMessage]"},
		{name: "unknown", typeName: "geometry", want: "any"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			column := &plugin.Column{
				Type:     &plugin.Identifier{Name: test.typeName},
				NotNull:  test.notNull,
				Unsigned: test.unsigned,
				Length:   test.length,
			}
			if got := getType(column); got != test.want {
				t.Errorf("getType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormaliseEnumTypesRenamesOnlyTheCatalogsEnums(t *testing.T) {
	enumColumn := func(schema string, name string) *plugin.Column {
		return &plugin.Column{Type: &plugin.Identifier{Schema: schema, Name: name}}
	}
	tableColumn := enumColumn("", "things_kind")
	otherSchemaColumn := enumColumn("archive", "things_kind")
	queryColumn := enumColumn("", "things_kind")
	parameterColumn := enumColumn("", "things_kind")
	plainColumn := enumColumn("", "varchar")
	request := &plugin.GenerateRequest{
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{{
				Name:  "public",
				Enums: []*plugin.Enum{{Name: "things_kind"}},
				Tables: []*plugin.Table{{
					Rel:     &plugin.Identifier{Name: "things"},
					Columns: []*plugin.Column{tableColumn, otherSchemaColumn, plainColumn, {Name: "untyped"}},
				}},
			}},
		},
		Queries: []*plugin.Query{{
			Columns: []*plugin.Column{queryColumn},
			Params:  []*plugin.Parameter{{Number: 1, Column: parameterColumn}},
		}},
	}

	normaliseEnumTypes(request)

	for name, column := range map[string]*plugin.Column{"table": tableColumn, "query": queryColumn, "parameter": parameterColumn} {
		if column.Type.Name != "enum" {
			t.Errorf("%s column type = %s, want enum", name, column.Type.Name)
		}
	}
	if otherSchemaColumn.Type.Name != "things_kind" || plainColumn.Type.Name != "varchar" {
		t.Errorf("renamed a column that holds no enum of the default schema")
	}

	// A catalog without enums is left alone.
	normaliseEnumTypes(&plugin.GenerateRequest{Catalog: &plugin.Catalog{}})
}

func TestGetColumnNameUsesPositionForUnnamedColumn(t *testing.T) {
	if got := getColumnName(&plugin.Column{}, 2); got != "column_3" {
		t.Errorf("getColumnName() = %q, want column_3", got)
	}
}

func TestColumnsToStructResolvesDuplicateFields(t *testing.T) {
	columns := []Column{
		{id: 1, Column: testPluginColumn("value", "geometry", true, true)},
		{id: 1, Column: testPluginColumn("value", "text", true, true)},
	}

	result, err := columnsToStruct(&Options{}, "Result", columns, true)
	if err != nil {
		t.Fatalf("columnsToStruct() error: %v", err)
	}
	for i, field := range result.Fields {
		if field.Name != "Value" || field.Type != "string" {
			t.Errorf("field %d = %#v, want Value string", i, field)
		}
	}
}

func TestColumnsToStructSuffixesDuplicateColumns(t *testing.T) {
	columns := []Column{
		{id: 1, Column: testPluginColumn("value", "text", true, false)},
		{id: 2, Column: testPluginColumn("value", "text", true, false)},
	}

	result, err := columnsToStruct(&Options{}, "Result", columns, false)
	if err != nil {
		t.Fatalf("columnsToStruct() error: %v", err)
	}
	if got := result.Fields[1].Name; got != "Value_2" {
		t.Errorf("second field name = %q, want Value_2", got)
	}
}

func TestColumnsToStructRejectsIncompatibleNamedParams(t *testing.T) {
	columns := []Column{
		{id: 1, Column: testPluginColumn("value", "text", true, true)},
		{id: 2, Column: testPluginColumn("value", "int", true, true)},
	}

	if _, err := columnsToStruct(&Options{}, "Params", columns, false); err == nil {
		t.Fatal("columnsToStruct() accepted incompatible named params")
	}
}

func testPluginColumn(name string, typeName string, notNull bool, named bool) *plugin.Column {
	return &plugin.Column{
		Name:         name,
		Type:         &plugin.Identifier{Name: typeName},
		NotNull:      notNull,
		IsNamedParam: named,
	}
}
