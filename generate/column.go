package generate

import (
	"fmt"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

type Column struct {
	*plugin.Column

	id    int
	embed *Struct
}

// findEmbed returns the model an sqlc.embed() column stands for. sqlc names only the table, so
// the model has to be found among the generated structs; a table with no model (a view, say, or
// one from another catalog) is an error rather than a field of type any that cannot be scanned.
func findEmbed(table *plugin.Identifier, structs []Struct, defaultSchema string) (*Struct, error) {
	schema := defaultSchema
	if table.Schema != "" {
		schema = table.Schema
	}
	for index := range structs {
		model := &structs[index]
		if table.Catalog == model.Table.Catalog && table.Name == model.Table.Name && schema == model.Table.Schema {
			return model, nil
		}
	}
	return nil, fmt.Errorf("sqlc.embed(%s): no model for table %s", table.Name, table.Name)
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
	// Only tinyint(1), which is also what boolean and bool are stored as, holds a flag, as
	// sqlc-gen-go reads it. Any other tinyint holds a number, and scanning a number above 1 into a
	// bool fails.
	case "tinyint":
		if column.Length != 1 {
			return integerType(notNull, unsigned)
		}
		if notNull {
			return "bool"
		} else {
			return "sql.Null[bool]"
		}
	case "int", "bigint", "bigint signed", "bigint unsigned", "integer", "smallint", "mediumint", "year":
		return integerType(notNull, unsigned)
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
	// A MySQL enum column's type is named after its table and column, and normaliseEnumTypes has
	// already renamed it to enum.
	case "enum":
		if notNull {
			return "string"
		} else {
			return "sql.Null[string]"
		}
	case "json":
		if notNull {
			return "json.RawMessage"
		} else {
			return "sql.Null[json.RawMessage]"
		}
	}

	return "any"
}

// integerType is every integer's type, whatever its width: int64, or uint64 when unsigned.
func integerType(notNull bool, unsigned bool) string {
	switch {
	case notNull && unsigned:
		return "uint64"
	case notNull:
		return "int64"
	case unsigned:
		return "sql.Null[uint64]"
	default:
		return "sql.Null[int64]"
	}
}

// normaliseEnumTypes renames the type of every column holding one of the catalog's enums to enum.
// sqlc names a MySQL enum column's type after its table and column (resources_category) and lists
// it among the schema's enums, rather than calling it an enum.
func normaliseEnumTypes(req *plugin.GenerateRequest) {
	enums := map[string]bool{}
	for _, schema := range req.GetCatalog().GetSchemas() {
		for _, enum := range schema.GetEnums() {
			enums[schema.GetName()+"."+enum.GetName()] = true
		}
	}
	if len(enums) == 0 {
		return
	}
	defaultSchema := req.GetCatalog().GetDefaultSchema()
	rename := func(column *plugin.Column) {
		if column == nil || column.GetType() == nil {
			return
		}
		schema := column.GetType().GetSchema()
		if schema == "" {
			schema = defaultSchema
		}
		if enums[schema+"."+column.GetType().GetName()] {
			column.Type = &plugin.Identifier{Name: "enum"}
		}
	}
	for _, schema := range req.GetCatalog().GetSchemas() {
		for _, table := range schema.GetTables() {
			for _, column := range table.GetColumns() {
				rename(column)
			}
		}
	}
	for _, query := range req.GetQueries() {
		for _, column := range query.GetColumns() {
			rename(column)
		}
		for _, parameter := range query.GetParams() {
			rename(parameter.GetColumn())
		}
	}
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
		// An embedded table becomes one field named after its model, as sqlc-gen-go names it. The
		// model's name is already an identifier, and reading it as a column name would lower-case
		// every word after the first (CheckHost would become Checkhost).
		var fieldName string
		if column.embed != nil {
			fieldName = column.embed.Name
		} else {
			fieldName = getIdentifierName(getColumnName(column.Column, i), options)
		}
		baseFieldName := fieldName
		suffix := 0

		if o, ok := suffixes[column.id]; ok && useID {
			suffix = o
		} else if v := len(seen[baseFieldName]); v > 0 && !column.IsNamedParam {
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
		if column.embed != nil {
			f.Type = column.embed.Name
			f.EmbedFields = column.embed.Fields
			f.Nullable = false
		} else {
			f.Type = getGoType(column.Column)
		}

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
