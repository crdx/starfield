package starfield

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/inflection"
	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

var camelPattern = regexp.MustCompile("[^A-Z][A-Z]+")

type column struct {
	id int
	*plugin.Column
}

func tagsToString(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	tagParts := make([]string, 0, len(tags))
	for key, val := range tags {
		tagParts = append(tagParts, fmt.Sprintf("%s:%q", key, val))
	}
	slices.Sort(tagParts)
	return strings.Join(tagParts, " ")
}

func toCamelCase(s string) string {
	return toCamelInitCase(s, false)
}

func toCamelInitCase(name string, initialUpper bool) string {
	out := ""
	for i, p := range strings.Split(name, "_") {
		if !initialUpper && i == 0 {
			out += p
			continue
		}
		if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

func toLowerCase(str string) string {
	if str == "" {
		return ""
	}
	if str == "ID" {
		return "id"
	}

	return strings.ToLower(str[:1]) + str[1:]
}

func trimSliceAndPointerPrefix(v string) string {
	v = strings.TrimPrefix(v, "[]")
	v = strings.TrimPrefix(v, "*")
	return v
}

func hasPrefixIgnoringSliceAndPointerPrefix(s, prefix string) bool {
	trimmedS := trimSliceAndPointerPrefix(s)
	trimmedPrefix := trimSliceAndPointerPrefix(prefix)
	return strings.HasPrefix(trimmedS, trimmedPrefix)
}

func fixNamingConflicts(imports []string, queries []Query) []Query {
	m := make(map[string]struct{})
	for _, path := range imports {
		paths := strings.Split(path, "/")
		m[paths[len(paths)-1]] = struct{}{}
	}

	replacedQueries := make([]Query, 0, len(queries))
	for _, query := range queries {
		if _, exist := m[query.Argument.Name]; exist {
			query.Argument.Name = toCamelCase(fmt.Sprintf("arg_%s", query.Argument.Name))
		}
		replacedQueries = append(replacedQueries, query)
	}
	return replacedQueries
}

func log(obj ...any) {
	logS(spew.Sdump(obj...))
}

func logF(str string, args ...any) {
	logS(fmt.Sprintf(str, args...))
}

func logS(str string) {
	file := lo.Must(os.OpenFile("/tmp/starlog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666))
	defer file.Close()
	lo.Must(file.WriteString(str + "\n"))
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

func escape(s string) string {
	if isReserved(s) {
		return s + "_"
	}
	return s
}

func isReserved(s string) bool {
	switch s {
	case "break":
		return true
	case "default":
		return true
	case "func":
		return true
	case "interface":
		return true
	case "select":
		return true
	case "case":
		return true
	case "defer":
		return true
	case "go":
		return true
	case "map":
		return true
	case "struct":
		return true
	case "chan":
		return true
	case "else":
		return true
	case "goto":
		return true
	case "package":
		return true
	case "switch":
		return true
	case "const":
		return true
	case "fallthrough":
		return true
	case "if":
		return true
	case "range":
		return true
	case "type":
		return true
	case "continue":
		return true
	case "for":
		return true
	case "import":
		return true
	case "return":
		return true
	case "var":
		return true
	default:
		return false
	}
}

func getColumnName(c *plugin.Column, pos int) string {
	if c.Name != "" {
		return c.Name
	}
	return fmt.Sprintf("column_%d", pos+1)
}

func getParamName(p *plugin.Parameter) string {
	if p.Column.Name != "" {
		return getArgName(p.Column.Name)
	}
	return fmt.Sprintf("dollar_%d", p.Number)
}

func getArgName(name string) string {
	if !strings.Contains(name, "_") {
		return name
	}

	out := ""
	for i, p := range strings.Split(name, "_") {
		if i == 0 {
			out += strings.ToLower(p)
		} else if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

func buildQueries(req *plugin.GenerateRequest, options *Options, structs []Struct) ([]Query, error) {
	queries := make([]Query, 0, len(req.Queries))

	for _, sourceQuery := range req.Queries {
		if sourceQuery.Name == "" {
			continue
		}
		if sourceQuery.Cmd == "" {
			continue
		}

		query := Query{
			Command:      sourceQuery.Cmd,
			ConstantName: sdk.LowerTitle(sourceQuery.Name),
			MethodName:   sourceQuery.Name,
			SourceName:   strings.TrimSuffix(sourceQuery.Filename, ".sql"),
			SQL:          sourceQuery.Text,
			Comments:     sourceQuery.Comments,
		}

		queryParameterLimit := options.MaxParams.MustGet()

		if len(sourceQuery.Params) == 1 && queryParameterLimit != 0 {
			p := sourceQuery.Params[0]
			query.Argument = QueryValue{
				Name:   escape(getParamName(p)),
				Typ:    getGoType(p.Column),
				Column: p.Column,
			}
		} else if len(sourceQuery.Params) >= 1 {
			var cols []column
			for _, p := range sourceQuery.Params {
				cols = append(cols, column{
					id:     int(p.Number),
					Column: p.Column,
				})
			}
			s, err := columnsToStruct(options, query.MethodName+"Params", cols, false)
			if err != nil {
				return nil, err
			}
			query.Argument = QueryValue{
				Emit:   true,
				Name:   "arg",
				Struct: s,
			}
			if len(sourceQuery.Params) <= queryParameterLimit {
				query.Argument.Emit = false
			}
		}

		if len(sourceQuery.Columns) == 1 {
			column := sourceQuery.Columns[0]
			name := getColumnName(column, 0)
			name = strings.Replace(name, "$", "_", -1)
			query.ReturnValue = QueryValue{
				Name: escape(name),
				Typ:  getGoType(column),
			}
		} else if returnsData(sourceQuery) {
			var gs *Struct
			var emit bool

			for _, s := range structs {
				if len(s.Fields) != len(sourceQuery.Columns) {
					continue
				}
				same := true
				for i, f := range s.Fields {
					column := sourceQuery.Columns[i]
					sameName := f.Name == getStructName(getColumnName(column, i), options)
					sameType := f.Type == getGoType(column)
					sameTable := sdk.SameTableName(column.Table, s.Table, req.Catalog.DefaultSchema)
					if !sameName || !sameType || !sameTable {
						same = false
					}
				}
				if same {
					gs = &s
					break
				}
			}

			if gs == nil {
				var columns []column
				for i, c := range sourceQuery.Columns {
					columns = append(columns, column{
						id:     i,
						Column: c,
					})
				}
				var err error
				gs, err = columnsToStruct(options, query.MethodName+"Row", columns, true)
				if err != nil {
					return nil, err
				}
				emit = true
			}
			query.ReturnValue = QueryValue{
				Emit:   emit,
				Name:   "item",
				Struct: gs,
			}
		}

		queries = append(queries, query)
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].MethodName < queries[j].MethodName })
	return queries, nil
}

func returnsData(query *plugin.Query) bool {
	return query.Cmd == metadata.CmdMany || query.Cmd == metadata.CmdOne
}

func columnsToStruct(options *Options, name string, columns []column, useID bool) (*Struct, error) {
	gs := Struct{
		Name: name,
	}
	seen := map[string][]int{}
	suffixes := map[int]int{}
	for i, column := range columns {
		columnName := getColumnName(column.Column, i)

		fieldName := getStructName(columnName, options)
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

func checkIncompatibleFieldTypes(fields []Field) error {
	fieldTypes := map[string]string{}
	for _, field := range fields {
		if fieldType, found := fieldTypes[field.Name]; !found {
			fieldTypes[field.Name] = field.Type
		} else if field.Type != fieldType {
			return fmt.Errorf("named param %s has incompatible types: %s, %s", field.Name, field.Type, fieldType)
		}
	}
	return nil
}

func buildStructs(req *plugin.GenerateRequest, options *Options) []Struct {
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

			structName := toSingular(tableName, options.PreserveTables)
			s := Struct{
				Table:   &plugin.Identifier{Schema: schema.Name, Name: table.Rel.Name},
				Name:    getStructName(structName, options),
				Comment: table.Comment,
				IsView:  strings.HasSuffix(tableName, "_view"),
			}

			for _, column := range table.Columns {
				tags := map[string]string{
					"column": column.Name,
				}
				s.Fields = append(s.Fields, Field{
					Nullable: !column.NotNull,
					Name:     getStructName(column.Name, options),
					Type:     getGoType(column),
					Tags:     tags,
					Column:   column,
				})
			}

			fieldNames := lo.Map(s.Fields, func(field Field, _ int) string {
				return field.Column.Name
			})

			s.FieldNames = strings.Join(fieldNames, ", ")
			s.HasDeletedAt = slices.Contains(fieldNames, "deleted_at")
			s.HasCreatedAt = slices.Contains(fieldNames, "created_at")
			s.HasID = slices.Contains(fieldNames, "id")
			s.Placeholders = strings.Join(fillSlice(len(s.Fields), "?"), ", ")

			structs = append(structs, s)
		}
	}
	if len(structs) > 0 {
		sort.Slice(structs, func(i, j int) bool { return structs[i].Name < structs[j].Name })
	}
	return structs
}

// fillSlice fills a slice with n instances of val.
func fillSlice(n int, s string) []string {
	slice := make([]string, n)
	for i := range slice {
		slice[i] = s
	}
	return slice
}

func getGoType(column *plugin.Column) string {
	innerType := getGoInnerType(column)
	if column.IsSqlcSlice {
		return "[]" + innerType
	}
	if column.IsArray {
		return strings.Repeat("[]", int(column.ArrayDims)) + innerType
	}
	return innerType
}

func getGoInnerType(col *plugin.Column) string {
	return getType(col)
}

func toSingular(s string, exclusions []string) string {
	for _, exclusion := range exclusions {
		if strings.EqualFold(s, exclusion) {
			return s
		}
	}

	return inflection.Singular(s)
}
