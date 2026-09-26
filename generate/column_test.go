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
		want     string
	}{
		{name: "string", typeName: "varchar", notNull: true, want: "string"},
		{name: "nullable string", typeName: "text", want: "sql.Null[string]"},
		{name: "boolean tinyint", typeName: "tinyint", notNull: true, want: "bool"},
		{name: "nullable tinyint", typeName: "tinyint", want: "sql.Null[bool]"},
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
		{name: "enum", typeName: "enum", want: "string"},
		{name: "unknown", typeName: "geometry", want: "any"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			column := &plugin.Column{
				Type:     &plugin.Identifier{Name: test.typeName},
				NotNull:  test.notNull,
				Unsigned: test.unsigned,
			}
			if got := getType(column); got != test.want {
				t.Errorf("getType() = %q, want %q", got, test.want)
			}
		})
	}
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
