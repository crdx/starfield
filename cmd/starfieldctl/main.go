package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"time"

	"crdx.org/col"
	"crdx.org/duckopt/v2"
	"crdx.org/starfield/scaffold"
	"github.com/samber/lo"
)

func getUsage() string {
	return `
		Usage:
			$0 [options] init
			$0 [options] make-migration <name>
	`
}

type Opts struct {
	Init          bool   `docopt:"init"`
	MakeMigration bool   `docopt:"make-migration"`
	Name          string `docopt:"<name>"`
}

const (
	schemaDir  = "db/schema"
	queriesDir = "db/queries"
	sqlcFile   = "sqlc.yml"
)

func main() {
	log.SetFlags(0)

	opts := duckopt.MustBind[Opts](getUsage(), "$0")

	if opts.Init {
		doInit()
		os.Exit(0)
	}

	if opts.MakeMigration {
		makeMigration(opts.Name)
		os.Exit(0)
	}
}

func doInit() {
	log.SetFlags(0)

	schemaPath := path.Join(schemaDir, "0000000000_init.sql")
	mainPath := path.Join(schemaDir, "main.go")
	queryPath := path.Join(queriesDir, "foos.sql")

	if isReadable(sqlcFile) {
		log.Printf(col.Yellow("skip %s"), sqlcFile)
	} else {
		log.Printf(col.Green("write %s"), sqlcFile)
		file := lo.Must(os.OpenFile(sqlcFile, os.O_CREATE|os.O_WRONLY, 0o644))
		lo.Must0(template.Must(template.New(sqlcFile).Parse(scaffold.SqlcYML)).Execute(
			file,
			map[string]string{"Name": path.Base(lo.Must(os.Getwd()))},
		))
	}

	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), schemaDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), schemaDir)
		if isReadable(schemaPath) {
			log.Printf(col.Yellow("skip %s"), schemaPath)
		} else {
			log.Printf(col.Green("write %s"), schemaPath)
			lo.Must0(os.WriteFile(schemaPath, scaffold.MigrationSQL, 0o644))
		}
		if isReadable(mainPath) {
			log.Printf(col.Yellow("skip %s"), mainPath)
		} else {
			log.Printf(col.Green("write %s"), mainPath)
			lo.Must0(os.WriteFile(mainPath, scaffold.MainGo, 0o644))
		}
	}

	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), queriesDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), queriesDir)
		if isReadable(queryPath) {
			log.Printf(col.Yellow("skip %s"), queryPath)
		} else {
			log.Printf(col.Green("write %s"), queryPath)
			lo.Must0(os.WriteFile(queryPath, scaffold.QuerySQL, 0o644))
		}
	}
}

func makeMigration(name string) {
	fileName := fmt.Sprintf("%d_%s", time.Now().UTC().Unix(), snakeCase(name)) + ".sql"

	// TODO(x): read from sqlc.yml itself instead of assuming the default
	filePath := filepath.Join(schemaDir, fileName)

	_, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("✓ %s", col.Green(fileName))
}
