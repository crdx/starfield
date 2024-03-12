package generate

import (
	"fmt"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

type Column struct {
	id int
	*plugin.Column
}

func getType(column *plugin.Column) string {
	columnType := sdk.DataType(column.Type)
	notNull := column.NotNull || column.IsArray
	unsigned := column.Unsigned

	switch columnType {
	case "varchar", "text", "char", "tinytext", "mediumtext", "longtext":
		if notNull {
			return "string"
		} else {
			return "sql.Null[string]"
		}
	case "tinyint":
		if notNull {
			return "bool"
		} else {
			return "sql.Null[bool]"
		}
	case "int", "bigint", "integer", "smallint", "mediumint", "year":
		if notNull {
			if unsigned {
				return "uint64"
			} else {
				return "int64"
			}
		} else {
			if unsigned {
				return "sql.Null[uint64]"
			} else {
				return "sql.Null[int64]"
			}
		}
	case "blob", "binary", "varbinary", "tinyblob", "mediumblob", "longblob":
		if notNull {
			return "[]byte"
		} else {
			return "sql.Null[string]"
		}
	case "double", "double precision", "real", "float":
		if notNull {
			return "float64"
		} else {
			return "sql.Null[float64]"
		}
	case "decimal", "dec", "fixed":
		if notNull {
			return "string"
		} else {
			return "sql.Null[string]"
		}
	case "date", "timestamp", "datetime", "time":
		if notNull {
			return "time.Time"
		} else {
			return "sql.Null[time.Time]"
		}
	case "boolean", "bool":
		if notNull {
			return "bool"
		} else {
			return "sql.Null[bool]"
		}
	case "enum":
		return "string"
	}

	return "any"
}

func getColumnName(c *plugin.Column, pos int) string {
	if c.Name != "" {
		return c.Name
	}
	return fmt.Sprintf("column_%d", pos+1)
}

func columnsToStruct(options *Options, name string, columns []Column, useID bool) (*Struct, error) {
	gs := Struct{
		Name: name,
	}
	seen := map[string][]int{}
	suffixes := map[int]int{}
	for i, column := range columns {
		columnName := getColumnName(column.Column, i)

		fieldName := getIdentifierName(columnName, options)
		baseFieldName := fieldName
		suffix := 0

		if o, ok := suffixes[column.id]; ok && useID {
			suffix = o
		} else if v := len(seen[fieldName]); v > 0 && !column.IsNamedParam {
			suffix = v + 1
		}

		suffixes[column.id] = suffix
		if suffix > 0 {
			fieldName = fmt.Sprintf("%s_%d", fieldName, suffix)
		}

		f := Field{
			Name:     fieldName,
			Column:   column.Column,
			Nullable: !column.NotNull,
		}
		f.Type = getGoType(column.Column)

		gs.Fields = append(gs.Fields, f)
		if _, found := seen[baseFieldName]; !found {
			seen[baseFieldName] = []int{i}
		} else {
			seen[baseFieldName] = append(seen[baseFieldName], i)
		}
	}

	for i, field := range gs.Fields {
		if len(seen[field.Name]) > 1 && field.Type == "any" {
			for _, j := range seen[field.Name] {
				if i == j {
					continue
				}
				otherField := gs.Fields[j]
				if otherField.Type != field.Type {
					field.Type = otherField.Type
				}
				gs.Fields[i] = field
			}
		}
	}

	err := checkIncompatibleFieldTypes(gs.Fields)
	if err != nil {
		return nil, err
	}

	return &gs, nil
}
