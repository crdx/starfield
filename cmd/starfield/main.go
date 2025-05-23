package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path"
	"text/template"

	"crdx.org/col"
	"crdx.org/starfield/generate"
	"crdx.org/starfield/scaffold"
	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/codegen"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "init" {
		doInit()
		os.Exit(0)
	}

	codegen.Run(generate.Run)
}

func doInit() {
	log.SetFlags(0)
	sqlc := "sqlc.yml"
	migrationsDir := "migrations"
	schema := "0000000000_schema.sql"
	list := "list.go"
	schemaPath := path.Join(migrationsDir, schema)
	listPath := path.Join(migrationsDir, list)
	queriesDir := "queries"
	query := "foos.sql"
	queryPath := path.Join(queriesDir, query)

	if readable(sqlc) {
		log.Printf(col.Yellow("skip %s"), sqlc)
	} else {
		log.Printf(col.Green("write %s"), sqlc)
		file := lo.Must(os.OpenFile(sqlc, os.O_CREATE|os.O_WRONLY, 0o644))
		lo.Must0(template.Must(template.New(sqlc).Parse(scaffold.SqlcYML)).Execute(
			file,
			map[string]string{"Name": path.Base(lo.Must(os.Getwd()))},
		))
	}

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), migrationsDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), migrationsDir)
		if readable(schemaPath) {
			log.Printf(col.Yellow("skip %s"), schemaPath)
		} else {
			log.Printf(col.Green("write %s/%s"), migrationsDir, schema)
			lo.Must0(os.WriteFile(schemaPath, scaffold.MigrationSQL, 0o644))
		}
		if readable(listPath) {
			log.Printf(col.Yellow("skip %s"), listPath)
		} else {
			log.Printf(col.Green("write %s/%s"), migrationsDir, list)
			lo.Must0(os.WriteFile(listPath, scaffold.ListGo, 0o644))
		}
	}

	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), queriesDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), queriesDir)
		if readable(queryPath) {
			log.Printf(col.Yellow("skip %s"), queryPath)
		} else {
			log.Printf(col.Green("write %s/%s"), queriesDir, query)
			lo.Must0(os.WriteFile(fmt.Sprintf("%s/%s", queriesDir, query), scaffold.QuerySQL, 0o644))
		}
	}
}

func readable(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
