package main

import (
	_ "embed"
	"log"
	"os"

	starfield "crdx.org/starfield/pkg"
	"github.com/samber/lo"
	"github.com/sqlc-dev/plugin-sdk-go/codegen"
)

//go:embed sqlc.yml.template
var sqlc []byte

//go:embed migration.sql.template
var migration []byte

func main() {
	log.SetFlags(0)

	if len(os.Args[0]) == 2 && os.Args[1] == "init" {
		if PathExists("sqlc.yml") {
			log.Printf("Not creating sqlc.yml as it already exists")
		} else {
			log.Printf("Writing sqlc.yml")
			lo.Must0(os.WriteFile("sqlc.yml", sqlc, os.ModePerm))
		}

		log.Printf("Creating migrations dir")
		if err := MakeOutputDir("migrations"); err != nil {
			log.Printf("Unable to create dir: %s", err)
			return
		}
		log.Printf("Writing migrations/0000000000_init.sql")
		lo.Must0(os.WriteFile("migrations/0000000000_init.sql", migration, os.ModePerm))
	} else {
		codegen.Run(starfield.Generate)
	}
}
