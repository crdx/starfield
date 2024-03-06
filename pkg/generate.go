package starfield

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"go/format"
	"text/template"

	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
)

//go:embed templates/*
var templates embed.FS

type TemplateArgs struct {
	Package    string
	Structs    []Struct
	Queries    []Query
	Version    string
	SourceName string
	Imports    []string
}

type File struct {
	Name     string
	Template string
}

func Generate(ctx context.Context, req *plugin.GenerateRequest) (*plugin.GenerateResponse, error) {
	options, err := parseOptions(req)
	if err != nil {
		return nil, err
	}

	structs := makeStructs(req, options)
	queries, err := makeQueries(req, options, structs)
	if err != nil {
		return nil, err
	}

	return generate(req, options, structs, queries)
}

func generate(req *plugin.GenerateRequest, options *Options, structs []Struct, queries []Query) (*plugin.GenerateResponse, error) {
	importer := &Importer{
		Options: options,
		Queries: queries,
		Structs: structs,
	}

	funcMap := template.FuncMap{
		"comment":        sdk.DoubleSlashComment,
		"escape":         sdk.EscapeBacktick,
		"getMethod":      getMethod,
		"getReturnValue": getReturnValue,
	}

	templateEngine := template.Must(
		template.New("starfield").Funcs(funcMap).ParseFS(templates, "templates/*.tmpl"),
	)

	files := []File{
		{Name: "main", Template: "main"},
	}

	for _, query := range queries {
		files = append(files, File{
			Name:     query.SourceName,
			Template: "queries",
		})
	}

	files = lo.UniqBy(files, func(file File) string {
		return file.Name
	})

	var imports []string
	for _, importSpecs := range mergeImports(importer.mainImports(), importer.queryImports()) {
		for _, importSpec := range importSpecs {
			imports = append(imports, importSpec)
		}
	}

	imports = lo.Uniq(imports)
	queries = fixNamingConflicts(imports, queries)

	args := TemplateArgs{
		Package: options.Package,
		Structs: structs,
		Imports: imports,
		Version: req.SqlcVersion,
	}

	var snippets [][]byte

	render := func(file File) error {
		args.Queries = lo.Filter(queries, func(item Query, _ int) bool {
			return item.SourceName == file.Name
		})

		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)

		if err := templateEngine.ExecuteTemplate(writer, file.Template, &args); err != nil {
			return err
		}

		if err := writer.Flush(); err != nil {
			return err
		}

		code, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("unable to format %s from template %s: %w", file.Name, file.Template, err)
		}

		snippets = append(snippets, code)
		return nil
	}

	for _, file := range files {
		if err := render(file); err != nil {
			return nil, err
		}
	}

	code, err := format.Source(bytes.Join(snippets, []byte("\n")))
	if err != nil {
		return nil, fmt.Errorf("unable to format main file: %w", err)
	}

	response := plugin.GenerateResponse{}
	response.Files = append(response.Files, &plugin.File{
		Name:     "db.go",
		Contents: []byte(code),
	})

	return &response, nil
}
