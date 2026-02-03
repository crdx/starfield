package generate

import (
	"cmp"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

type Struct struct {
	Table        *plugin.Identifier
	Name         string
	Fields       []Field
	Comment      string
	IsView       bool
	FieldNames   string
	Placeholders string

	HasDeletedAt         bool
	HasCreatedAt         bool
	HasNullableCreatedAt bool
	HasID                bool
	HasUnsignedID        bool
}

func makeStructs(req *plugin.GenerateRequest, options *Options) []Struct {
	var structs []Struct
	for _, schema := range req.Catalog.Schemas {
		if schema.Name == "information_schema" {
			continue
		}
		for _, table := range schema.Tables {
			var tableName string
			if schema.Name == req.Catalog.DefaultSchema {
				tableName = table.Rel.Name
			} else {
				tableName = schema.Name + "_" + table.Rel.Name
			}

			structName := toSingular(tableName, options.Preserve)
			s := Struct{
				Table:   &plugin.Identifier{Schema: schema.Name, Name: table.Rel.Name},
				Name:    getIdentifierName(structName, options),
				Comment: table.Comment,
				IsView:  strings.HasSuffix(tableName, "_view"),
			}

			for _, column := range table.Columns {
				tags := map[string]string{
					"column": column.Name,
				}
				s.Fields = append(s.Fields, Field{
					Nullable: !column.NotNull,
					Name:     getIdentifierName(column.Name, options),
					Type:     getGoType(column),
					Tags:     tags,
					Column:   column,
				})
			}

			fieldMap := lo.SliceToMap(s.Fields, func(field Field) (string, Field) {
				return field.Column.Name, field
			})

			fieldNames := lo.Map(s.Fields, func(field Field, _ int) string {
				return field.Column.Name
			})

			s.FieldNames = "`" + strings.Join(fieldNames, "`, `") + "`"
			s.HasDeletedAt = slices.Contains(fieldNames, "deleted_at")
			s.HasNullableCreatedAt = fieldMap["created_at"].Nullable
			s.HasCreatedAt = slices.Contains(fieldNames, "created_at")
			s.HasID = slices.Contains(fieldNames, "id")
			s.HasUnsignedID = s.HasID && fieldMap["id"].Column.Unsigned
			s.Placeholders = strings.Join(fillSlice(len(s.Fields), "?"), ", ")

			structs = append(structs, s)
		}
	}

	if len(structs) > 0 {
		slices.SortFunc(structs, func(a, b Struct) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	return structs
}
