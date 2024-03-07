package starfield

import (
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

type Query struct {
	Command      string
	Comments     []string
	MethodName   string
	FieldName    string
	ConstantName string
	SourceName   string
	SQL          string
	ReturnValue  QueryValue
	Argument     QueryValue
}

func (self Query) hasRetType() bool {
	scanned := self.Command == metadata.CmdOne || self.Command == metadata.CmdMany
	return scanned && !self.ReturnValue.isEmpty()
}

type QueryValue struct {
	EmitAsStruct bool
	Name         string
	Struct       *Struct
	Typ          string
	Column       *plugin.Column
}

type Argument struct {
	Name string
	Type string
}

func (self QueryValue) EmitStruct() bool {
	return self.EmitAsStruct
}

func (self QueryValue) IsStruct() bool {
	return self.Struct != nil
}

func (self QueryValue) isEmpty() bool {
	return self.Typ == "" && self.Name == "" && self.Struct == nil
}

func (self QueryValue) Pair() string {
	var out []string
	for _, arg := range self.Pairs() {
		out = append(out, arg.Name+" "+arg.Type)
	}
	return strings.Join(out, ",")
}

func (self QueryValue) Pairs() []Argument {
	if self.isEmpty() {
		return nil
	}
	if !self.EmitStruct() && self.IsStruct() {
		var out []Argument
		for _, f := range self.Struct.Fields {
			out = append(out, Argument{
				Name: escape(toLowerCase(f.Name)),
				Type: f.Type,
			})
		}
		return out
	}
	return []Argument{
		{
			Name: escape(toLowerCase(self.Name)),
			Type: self.DefineType(),
		},
	}
}

func (self QueryValue) SlicePair() string {
	if self.isEmpty() {
		return ""
	}
	return self.Name + " []" + self.DefineType()
}

func (self QueryValue) Type() string {
	if self.Typ != "" {
		return self.Typ
	}
	if self.Struct != nil {
		return self.Struct.Name
	}
	panic("no type for QueryValue: " + self.Name)
}

func (self *QueryValue) DefineType() string {
	return self.Type()
}

func (self *QueryValue) ReturnName() string {
	return escape(self.Name)
}

func (self QueryValue) UniqueFields() []Field {
	seen := map[string]struct{}{}
	fields := make([]Field, 0, len(self.Struct.Fields))

	for _, field := range self.Struct.Fields {
		if _, found := seen[field.Name]; found {
			continue
		}
		seen[field.Name] = struct{}{}
		fields = append(fields, field)
	}

	return fields
}

func (self QueryValue) Params() string {
	if self.isEmpty() {
		return ""
	}
	var out []string
	if self.Struct == nil {
		out = append(out, escape(self.Name))
	} else {
		for _, f := range self.Struct.Fields {
			out = append(out, escape(self.VariableForField(f)))
		}
	}
	if len(out) <= 3 {
		return strings.Join(out, ",")
	}
	out = append(out, "")
	return "\n" + strings.Join(out, ",\n")
}

func (self QueryValue) HasSlices() bool {
	if self.Struct == nil {
		return self.Column != nil && self.Column.IsSqlcSlice
	}
	for _, v := range self.Struct.Fields {
		if v.Column.IsSqlcSlice {
			return true
		}
	}
	return false
}

func (self QueryValue) Scan() string {
	var out []string
	if self.Struct == nil {
		out = append(out, "&"+self.Name)
	} else {
		for _, f := range self.Struct.Fields {
			out = append(out, "&"+self.Name+"."+f.Name)
		}
	}
	if len(out) <= 3 {
		return strings.Join(out, ",")
	}
	out = append(out, "")
	return "\n" + strings.Join(out, ",\n")
}

func (self QueryValue) VariableForField(f Field) string {
	if !self.IsStruct() {
		return self.Name
	}
	if !self.EmitStruct() {
		return toLowerCase(f.Name)
	}
	return self.Name + "." + f.Name
}

func makeQueries(req *plugin.GenerateRequest, options *Options, structs []Struct) ([]Query, error) {
	queries := make([]Query, 0, len(req.Queries))

	for _, sourceQuery := range req.Queries {
		query := Query{
			Command:      sourceQuery.Cmd,
			ConstantName: sdk.LowerTitle(sourceQuery.Name),
			MethodName:   sourceQuery.Name,
			SourceName:   strings.TrimSuffix(sourceQuery.Filename, ".sql"),
			SQL:          sourceQuery.Text,
			Comments:     sourceQuery.Comments,
		}

		maxParams := options.MaxParams.MustGet()

		if len(sourceQuery.Params) > 0 {
			var columns []Column
			for _, param := range sourceQuery.Params {
				columns = append(columns, Column{
					id:     int(param.Number),
					Column: param.Column,
				})
			}
			s, err := columnsToStruct(options, query.MethodName+"Params", columns, false)
			if err != nil {
				return nil, err
			}

			query.Argument = QueryValue{
				EmitAsStruct: shouldEmitAsStruct(sourceQuery.Params, maxParams),
				Name:         "arg",
				Struct:       s,
			}
		}

		if len(sourceQuery.Columns) == 1 {
			column := sourceQuery.Columns[0]
			query.ReturnValue = QueryValue{
				Name: escape(getColumnName(column, 0)),
				Typ:  getGoType(column),
			}
		} else if returnsData(sourceQuery) {
			var gs *Struct
			var emitAsStruct bool

			for _, s := range structs {
				if len(s.Fields) != len(sourceQuery.Columns) {
					continue
				}
				same := true
				for i, f := range s.Fields {
					column := sourceQuery.Columns[i]
					sameName := f.Name == getIdentifierName(getColumnName(column, i), options)
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
				var columns []Column
				for i, c := range sourceQuery.Columns {
					columns = append(columns, Column{
						id:     i,
						Column: c,
					})
				}
				var err error
				gs, err = columnsToStruct(options, query.MethodName+"Row", columns, true)
				if err != nil {
					return nil, err
				}
				emitAsStruct = true
			}
			query.ReturnValue = QueryValue{
				EmitAsStruct: emitAsStruct,
				Name:         "item",
				Struct:       gs,
			}
		}

		queries = append(queries, query)
	}

	sort.Slice(queries, func(i, j int) bool {
		return queries[i].MethodName < queries[j].MethodName
	})

	return queries, nil
}

func shouldEmitAsStruct(params []*plugin.Parameter, maxParams int) bool {
	if maxParams < 0 {
		return false
	}

	// maxParams is a hard limit to the maximum number of function arguments.
	if len(params) > maxParams {
		return true
	}

	// Otherwise, as long as it's not a mix of updates and conditionals, go for it.
	updateCount, conditionalCount := classifyParams(params)
	return updateCount > 0 && conditionalCount > 0
}

func classifyParams(params []*plugin.Parameter) (updateCount int, conditionalCount int) {
	// This is probably an sqlc implementation detail, but for lack of a better way, this will have to
	// do.
	for _, param := range params {
		if param.Column.Table == nil {
			conditionalCount++
		} else if param.Column.Table.Schema != "" {
			updateCount++
		} else {
			conditionalCount++
		}
	}
	return
}

func getMethod(query Query) string {
	switch query.Command {
	case ":one":
		return "QueryRow"
	case ":many":
		return "Query"
	default:
		return "Exec"
	}
}

func getReturnValue(query Query) string {
	switch query.Command {
	case ":one":
		return "row :="
	case ":many":
		return "rows, err :="
	case ":exec":
		return "_, err :="
	case ":execrows", ":execlastid":
		return "result, err :="
	case ":execresult":
		return "result, err :="
	}
	return ""
}

func returnsData(query *plugin.Query) bool {
	return query.Cmd == metadata.CmdMany || query.Cmd == metadata.CmdOne
}
