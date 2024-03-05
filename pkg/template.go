package starfield

import "embed"

//go:embed templates/*
var templates embed.FS

type templateArgs struct {
	Package    string
	Structs    []Struct
	Queries    []Query
	Version    string
	SourceName string
	Imports    []string
}

func getMethod(q Query) string {
	switch q.Command {
	case ":one":
		return "QueryRow"
	case ":many":
		return "Query"
	default:
		return "Exec"
	}
}

func getReturnValue(q Query) string {
	switch q.Command {
	case ":one":
		return "row :="
	case ":many":
		return "rows, err :="
	case ":exec":
		return "_, err :="
	case ":execrows", ":execlastid":
		return "result, err :="
	case ":execresult":
		return "result, err :="
	}
	return ""
}
