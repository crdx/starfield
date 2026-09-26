package generate

import (
	"reflect"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestUtilityFormatting(t *testing.T) {
	if got := formatTags(nil); got != "" {
		t.Errorf("formatTags(nil) = %q, want empty", got)
	}
	if got := formatTags(map[string]string{"json": "name", "column": "full_name"}); got != `column:"full_name" json:"name"` {
		t.Errorf("formatTags() = %q", got)
	}
	if got := toCamelCase("foo_id_name"); got != "fooIDName" {
		t.Errorf("toCamelCase() = %q, want fooIDName", got)
	}

	lowercaseTests := map[string]string{
		"":     "",
		"ID":   "id",
		"UUID": "uuid",
		"Name": "name",
	}
	for input, want := range lowercaseTests {
		if got := toLowerCase(input); got != want {
			t.Errorf("toLowerCase(%q) = %q, want %q", input, got, want)
		}
	}

	if got := trimSliceAndPointerPrefix("[]*Thing"); got != "Thing" {
		t.Errorf("trimSliceAndPointerPrefix() = %q, want Thing", got)
	}
	if !hasPrefixIgnoringSliceAndPointerPrefix("[]*time.Time", "time") {
		t.Error("hasPrefixIgnoringSliceAndPointerPrefix() = false, want true")
	}
}

func TestIdentifierUtilities(t *testing.T) {
	options := &Options{Rename: map[string]string{"api_url": "APIURL"}}
	tests := map[string]string{
		"api_url":  "APIURL",
		"thing_id": "ThingID",
		"9-lives":  "_9Lives",
	}
	for input, want := range tests {
		if got := getIdentifierName(input, options); got != want {
			t.Errorf("getIdentifierName(%q) = %q, want %q", input, got, want)
		}
	}

	if got := escape("type"); got != "type_" {
		t.Errorf("escape(type) = %q, want type_", got)
	}
	if got := escape("value"); got != "value" {
		t.Errorf("escape(value) = %q, want value", got)
	}
	if got := escapeVariable("rows"); got != "rows_" {
		t.Errorf("escapeVariable(rows) = %q, want rows_", got)
	}
}

func TestTypeAndInflectionUtilities(t *testing.T) {
	sliceColumn := testPluginColumn("ids", "int", true, false)
	sliceColumn.IsSqlcSlice = true
	if got := getGoType(sliceColumn); got != "[]int64" {
		t.Errorf("slice type = %q, want []int64", got)
	}

	arrayColumn := testPluginColumn("matrix", "int", false, false)
	arrayColumn.IsArray = true
	arrayColumn.ArrayDims = 2
	if got := getGoType(arrayColumn); got != "[][]int64" {
		t.Errorf("array type = %q, want [][]int64", got)
	}

	if got := toSingular("metadata", []string{"metadata"}); got != "metadata" {
		t.Errorf("preserved singular = %q, want metadata", got)
	}
	if got := toSingular("people", nil); got != "person" {
		t.Errorf("singular = %q, want person", got)
	}
	if got := toPlural("metadata", []string{"metadata"}); got != "metadataList" {
		t.Errorf("preserved plural = %q, want metadataList", got)
	}
	if got := toPlural("person", nil); got != "people" {
		t.Errorf("plural = %q, want people", got)
	}

	if got := oneline("one\n  two\tthree"); got != "one two three" {
		t.Errorf("oneline() = %q", got)
	}
}

func TestFillSlice(t *testing.T) {
	if got := fillSlice(3, "?"); !reflect.DeepEqual(got, []string{"?", "?", "?"}) {
		t.Errorf("fillSlice() = %#v", got)
	}
}

func TestFieldUtilities(t *testing.T) {
	nullable := Field{Type: "sql.Null[time.Time]", Tags: map[string]string{"column": "created_at"}, Column: &plugin.Column{IsSqlcSlice: true}}
	if got := nullable.BaseType(); got != "time.Time" {
		t.Errorf("BaseType() = %q, want time.Time", got)
	}
	if got := (Field{Type: "string"}).BaseType(); got != "string" {
		t.Errorf("BaseType() = %q, want string", got)
	}
	if got := nullable.Tag(); got != `column:"created_at"` {
		t.Errorf("Tag() = %q", got)
	}
	if !nullable.HasSlice() {
		t.Error("HasSlice() = false, want true")
	}
}
