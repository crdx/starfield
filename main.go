package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path"
	"text/template"

	"crdx.org/col"
	starfield "crdx.org/starfield/pkg"
	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/codegen"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "init" {
		doInit()
		os.Exit(0)
	}

	codegen.Run(starfield.Generate)
}

//go:embed sqlc.yml.template
var sqlcTemplate string

//go:embed migration.sql.template
var migrationTemplate []byte

//go:embed query.sql.template
var queryTemplate []byte

func doInit() {
	log.SetFlags(0)
	sqlc := "sqlc.yml"
	migrationsDir := "src/migrations"
	schema := "0000000000_schema.sql"
	queriesDir := "queries"
	query := "foos.sql"

	if pathExists(sqlc) {
		log.Printf(col.Yellow("skip %s"), sqlc)
	} else {
		log.Printf(col.Green("write %s"), sqlc)
		file := lo.Must(os.OpenFile(sqlc, os.O_CREATE|os.O_WRONLY, 0o644))
		lo.Must0(template.Must(template.New(sqlc).Parse(sqlcTemplate)).Execute(
			file,
			map[string]string{"Name": path.Base(lo.Must(os.Getwd()))},
		))
	}

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), migrationsDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), migrationsDir)
		log.Printf(col.Green("write %s/%s"), migrationsDir, schema)
		lo.Must0(os.WriteFile(fmt.Sprintf("%s/%s", migrationsDir, schema), migrationTemplate, 0o644))
	}

	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), queriesDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), queriesDir)
		log.Printf(col.Green("write %s/%s"), queriesDir, query)
		lo.Must0(os.WriteFile(fmt.Sprintf("%s/%s", queriesDir, query), queryTemplate, 0o644))
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
