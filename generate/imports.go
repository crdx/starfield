package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
)

var stdlibTypes = map[string]string{
	"sql.Null[time.Time]": "time",
	"time.Time":           "time",
}

type FileImports struct {
	Std []string
	Dep []string
}

type Importer struct {
	Options *Options
	Queries []Query
	Structs []Struct
}

func (self *Importer) usesType(typ string) bool {
	for _, strct := range self.Structs {
		for _, f := range strct.Fields {
			if hasPrefixIgnoringSliceAndPointerPrefix(f.Type, typ) {
				return true
			}
		}
	}
	return false
}

func (self *Importer) mainImports() FileImports {
	std, pkg := buildImports(nil, self.usesType)

	std["bytes"] = true
	std["database/sql"] = true
	std["errors"] = true
	std["fmt"] = true
	std["log"] = true
	std["net/url"] = true
	std["os"] = true
	std["reflect"] = true
	std["regexp"] = true
	std["strings"] = true
	std["time"] = true
	std["runtime"] = true
	std["path/filepath"] = true

	return sortImports(std, pkg)
}

func (self *Importer) queryImports() FileImports {
	std, pkg := buildImports(self.Queries, func(name string) bool {
		for _, query := range self.Queries {
			if query.hasRetType() {
				if query.ReturnValue.EmitStruct() {
					for _, f := range query.ReturnValue.Struct.Fields {
						if hasPrefixIgnoringSliceAndPointerPrefix(f.Type, name) {
							return true
						}
					}
				}
				if hasPrefixIgnoringSliceAndPointerPrefix(query.ReturnValue.Type(), name) {
					return true
				}
			}

			if query.Argument.EmitStruct() {
				for _, f := range query.Argument.Struct.Fields {
					if hasPrefixIgnoringSliceAndPointerPrefix(f.Type, name) {
						return true
					}
				}
			}

			for _, f := range query.Argument.Pairs() {
				if hasPrefixIgnoringSliceAndPointerPrefix(f.Type, name) {
					return true
				}
			}
		}
		return false
	})

	hasSlices := func() bool {
		for _, query := range self.Queries {
			if query.Argument.HasSlices() {
				return true
			}
		}
		return false
	}

	if hasSlices() {
		std["strings"] = true
	}

	return sortImports(std, pkg)
}

func buildImports(queries []Query, uses func(string) bool) (map[string]bool, map[string]bool) {
	pkg := map[string]bool{}
	std := map[string]bool{}

	if uses("sql.Null") {
		std["database/sql"] = true
	}

	for _, q := range queries {
		if q.Command == metadata.CmdExecResult || q.Command == metadata.CmdOne {
			std["database/sql"] = true
		}
	}

	for typeName, pkg := range stdlibTypes {
		if uses(typeName) {
			std[pkg] = true
		}
	}

	return std, pkg
}

func sortImports(std map[string]bool, pkg map[string]bool) FileImports {
	var pkgs []string
	for spec := range pkg {
		pkgs = append(pkgs, spec)
	}
	var stds []string
	for path := range std {
		stds = append(stds, path)
	}
	sort.Strings(stds)
	sort.Strings(pkgs)
	return FileImports{stds, pkgs}
}

func mergeImports(imports ...FileImports) [][]string {
	if len(imports) == 1 {
		return [][]string{
			imports[0].Std,
			imports[0].Dep,
		}
	}

	var stds, pkgs []string
	seenStd := map[string]bool{}
	seenPkg := map[string]bool{}
	for i := range imports {
		for _, path := range imports[i].Std {
			if _, ok := seenStd[path]; ok {
				continue
			}
			stds = append(stds, path)
			seenStd[path] = true
		}
		for _, path := range imports[i].Dep {
			if _, ok := seenPkg[path]; ok {
				continue
			}
			pkgs = append(pkgs, path)
			seenPkg[path] = true
		}
	}
	return [][]string{stds, pkgs}
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
