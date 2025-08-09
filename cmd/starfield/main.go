package main

import (
	_ "embed"

	"crdx.org/starfield/generate"
	"github.com/sqlc-dev/plugin-sdk-go/codegen"
)

func main() {
	codegen.Run(generate.Run)
}
