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

const Version = "v1.9.0"

func getUsage() string {
	return `
		Usage:
			$0 [options] init
			$0 [options] make-migration <name> [--unix]
			$0 [options] version

		Options:
			-u, --unix    Use legacy timestamp format in migration names
	`
}

type Opts struct {
	Init          bool   `docopt:"init"`
	MakeMigration bool   `docopt:"make-migration"`
	Version       bool   `docopt:"version"`
	Name          string `docopt:"<name>"`
	Unix          bool   `docopt:"--unix"`
}

const (
	sqlcFile = "sqlc.yml"
)

func main() {
	log.SetFlags(0)

	opts := duckopt.MustBind[Opts](getUsage(), "$0")

	if opts.Version {
		fmt.Println(Version)
		os.Exit(0)
	}

	if opts.Init {
		doInit()
		os.Exit(0)
	}

	if opts.MakeMigration {
		makeMigration(opts.Name, opts.Unix)
		os.Exit(0)
	}
}

func doInit() {
	log.SetFlags(0)

	schemaDir := "db/schema"
	queriesDir := "db/queries"

	schemaPath := path.Join(schemaDir, "00000000000000_init.sql")
	mainPath := path.Join(schemaDir, "main.go")
	queryPath := path.Join(queriesDir, "foos.sql")

	if exists(sqlcFile) {
		log.Printf(col.Yellow("skip %s"), sqlcFile)
	} else {
		log.Printf(col.Green("write %s"), sqlcFile)
		file := lo.Must(os.OpenFile(sqlcFile, os.O_CREATE|os.O_WRONLY, 0o600))
		lo.Must0(template.Must(template.New(sqlcFile).Parse(scaffold.SqlcYML)).Execute(
			file,
			map[string]string{"Name": path.Base(lo.Must(os.Getwd()))},
		))
	}

	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), schemaDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), schemaDir)
		if exists(schemaPath) {
			log.Printf(col.Yellow("skip %s"), schemaPath)
		} else {
			log.Printf(col.Green("write %s"), schemaPath)
			lo.Must0(os.WriteFile(schemaPath, scaffold.MigrationSQL, 0o600))
		}
		if exists(mainPath) {
			log.Printf(col.Yellow("skip %s"), mainPath)
		} else {
			log.Printf(col.Green("write %s"), mainPath)
			lo.Must0(os.WriteFile(mainPath, scaffold.MainGo, 0o600))
		}
	}

	if err := os.MkdirAll(queriesDir, 0o750); err != nil {
		log.Printf(col.Red("mkdir %s: %s"), queriesDir, err)
	} else {
		log.Printf(col.Green("mkdir %s"), queriesDir)
		if exists(queryPath) {
			log.Printf(col.Yellow("skip %s"), queryPath)
		} else {
			log.Printf(col.Green("write %s"), queryPath)
			lo.Must0(os.WriteFile(queryPath, scaffold.QuerySQL, 0o600))
		}
	}
}

func getMigrationID(name string, unix bool) string {
	name = snakeCase(name)

	if unix {
		return fmt.Sprintf("%d_%s", time.Now().UTC().Unix(), name)
	} else {
		return fmt.Sprintf("%s_%s", time.Now().UTC().Format("20060102150405"), name)
	}
}

func makeMigration(name string, unix bool) {
	schemaDir, err := getSchemaDir(sqlcFile)
	if err != nil {
		log.Fatal(err)
	}

	fileName := getMigrationID(name, unix) + ".sql"
	filePath := filepath.Join(schemaDir, fileName)

	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		outputMigrationMessage(false, fileName, "mkdir")
	}

	if exists(filePath) {
		outputMigrationMessage(false, fileName, "duplicate")
	}

	f, err := os.OpenFile(filepath.Clean(filePath), os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		outputMigrationMessage(false, fileName, err.Error())
	}

	_ = f.Close()
	outputMigrationMessage(true, fileName, "created")
}

func outputMigrationMessage(ok bool, fileName string, message string) {
	var icon string
	if ok {
		icon = col.Green("✓")
		fileName = col.Green(fileName)
	} else {
		icon = col.Red("𐄂")
		fileName = col.Red(fileName)
	}
	fmt.Printf("%s %s [%s]\n", icon, col.Red(fileName), message)
	if !ok {
		os.Exit(1)
	}
}
