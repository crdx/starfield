package starfield

import (
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
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
	Emit   bool
	Name   string
	Struct *Struct
	Typ    string
	Column *plugin.Column
}

type Argument struct {
	Name string
	Type string
}

func (self QueryValue) EmitStruct() bool {
	return self.Emit
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
