package scaffold

import _ "embed"

//go:embed sqlc.yml
var SqlcYML string

//go:embed migration.sql
var MigrationSQL []byte

//go:embed query.sql
var QuerySQL []byte
