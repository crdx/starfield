package generate

import (
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestMakeStructsSkipsInformationSchemaAndPrefixesOtherSchemas(t *testing.T) {
	request := &plugin.GenerateRequest{
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{
				{
					Name: "information_schema",
					Tables: []*plugin.Table{{
						Rel:     &plugin.Identifier{Name: "ignored"},
						Columns: []*plugin.Column{testPluginColumn("id", "int", true, false)},
					}},
				},
				{
					Name: "extra",
					Tables: []*plugin.Table{{
						Rel:     &plugin.Identifier{Name: "people"},
						Columns: []*plugin.Column{testPluginColumn("id", "int", true, false)},
					}},
				},
			},
		},
	}

	structs := makeStructs(request, &Options{})
	if len(structs) != 1 {
		t.Fatalf("makeStructs() returned %d structs, want 1", len(structs))
	}
	if got := structs[0].Name; got != "ExtraPerson" {
		t.Errorf("struct name = %q, want ExtraPerson", got)
	}
	if got := structs[0].Table.Schema; got != "extra" {
		t.Errorf("table schema = %q, want extra", got)
	}
}
