package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path"
	"text/template"

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

func doInit() {
	log.SetFlags(0)
	migrationsDir := "src/migrations"
	queriesDir := "queries"
	sqlc := "sqlc.yml"
	schema := "0000000000_schema.sql"

	if pathExists(sqlc) {
		log.Printf("\033[33mskip %s\033[0m", sqlc)
	} else {
		log.Printf("\033[32mwrite %s\033[0m", sqlc)
		file := lo.Must(os.OpenFile(sqlc, os.O_CREATE|os.O_WRONLY, 0o644))
		lo.Must0(template.Must(template.New(sqlc).Parse(sqlcTemplate)).Execute(
			file,
			map[string]string{"Name": path.Base(lo.Must(os.Getwd()))},
		))
	}

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		log.Printf("\033[31mmkdir %s: %s\033[0m", migrationsDir, err)
	} else {
		log.Printf("\033[32mwrite %s/%s\033[0m", migrationsDir, schema)
		lo.Must0(os.WriteFile(fmt.Sprintf("%s/%s", migrationsDir, schema), migrationTemplate, 0o644))
	}

	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		log.Printf("\033[31mmkdir %s: %s\033[0m", queriesDir, err)
	} else {
		log.Printf("\033[32mmkdir %s\033[0m", queriesDir)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
