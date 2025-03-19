package generate

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/inflection"
	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func formatTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tags))
	for k, v := range tags {
		parts = append(parts, fmt.Sprintf("%s:%q", k, v))
	}
	slices.Sort(parts)
	return strings.Join(parts, " ")
}

func toCamelCase(s string) string {
	out := ""
	for i, part := range strings.Split(s, "_") {
		if i == 0 {
			out += part
			continue
		}
		if part == "id" {
			out += "ID"
		} else {
			out += cases.Title(language.English).String(part)
		}
	}
	return out
}

func toLowerCase(s string) string {
	if s == "" {
		return ""
	}
	if s == "ID" {
		return "id"
	}
	if s == "UUID" {
		return "uuid"
	}

	return strings.ToLower(s[:1]) + s[1:]
}

func trimSliceAndPointerPrefix(v string) string {
	v = strings.TrimPrefix(v, "[]")
	v = strings.TrimPrefix(v, "*")
	return v
}

func hasPrefixIgnoringSliceAndPointerPrefix(s string, prefix string) bool {
	trimmedS := trimSliceAndPointerPrefix(s)
	trimmedPrefix := trimSliceAndPointerPrefix(prefix)
	return strings.HasPrefix(trimmedS, trimmedPrefix)
}

func log(obj ...any) { //nolint
	logS(spew.Sdump(obj...))
}

func logF(str string, args ...any) { //nolint
	logS(fmt.Sprintf(str, args...))
}

func logS(str string) { //nolint
	file := lo.Must(os.OpenFile("/tmp/starlog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666))
	defer file.Close()
	lo.Must(file.WriteString(str + "\n"))
}

func escape(s string) string {
	if isReserved(s) {
		return s + "_"
	}
	return s
}

func isReserved(s string) bool {
	keywords := []string{
		"break", "default", "func", "interface",
		"select", "case", "defer", "go",
		"map", "struct", "chan", "else",
		"goto", "package", "switch", "const",
		"fallthrough", "if", "range", "type",
		"continue", "for", "import", "return",
		"var",
	}
	return slices.Contains(keywords, s)
}

// fillSlice fills a slice with n instances of s.
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

func getGoInnerType(column *plugin.Column) string {
	return getType(column)
}

func toSingular(s string, exclusions []string) string {
	for _, exclusion := range exclusions {
		if strings.EqualFold(s, exclusion) {
			return s
		}
	}

	return inflection.Singular(s)
}

func getIdentifierName(name string, options *Options) string {
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
			out += cases.Title(language.English).String(p)
		}
	}

	r, _ := utf8.DecodeRuneInString(out)
	if unicode.IsDigit(r) {
		return "_" + out
	} else {
		return out
	}
}

func oneline(s string) string {
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}
