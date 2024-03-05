package starfield

import (
	"strings"
	"unicode"
	"unicode/utf8"

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

	HasDeletedAt bool
	HasCreatedAt bool
	HasID        bool
}

func getStructName(name string, options *Options) string {
	if rename := options.Rename[name]; rename != "" {
		return rename
	}
	out := ""
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return r
		}
		if unicode.IsDigit(r) {
			return r
		}
		return rune('_')
	}, name)

	for _, p := range strings.Split(name, "_") {
		if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}

	r, _ := utf8.DecodeRuneInString(out)
	if unicode.IsDigit(r) {
		return "_" + out
	} else {
		return out
	}
}
