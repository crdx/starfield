# starfield

**starfield** is a MySQL/SQLite sqlc plugin based on [sqlc-gen-go](https://github.com/sqlc-dev/sqlc-gen-go).

## Introduction

Read the [introductory post](https://textplain.org/starfield).

## Features

Compared to `sqlc-gen-go` the most notable change is the generation of a set of lookup and CRUD methods for each table in the schema. These are available without the need to define any queries yourself.

For a model `M`:

- `GetMTableStatus() (TableStatus, error)` returns an instance of `TableStatus` with information about the table.
- `CreateM(value) *M` creates a new `M` and returns itself with the last insert ID that was populated.
- `FindM(id) (*M, bool)` returns the non-deleted `M` with the specified ID, and whether it was found.
- `FindMUnscoped(id) (*M, bool)` returns the `M` with the specified ID, and whether it was found.
- `FindMs() []*M` returns all non-deleted `M`s in the database.
- `FindMsUnscoped() []*M` returns all `M`s in the database.

For a model `M` and a column `C`:

- `FindMsByC(value) []*M` finds all non-deleted `M`s with column `C` matching `value`.
- `FindMsByCUnscoped(value) []*M` finds all `M`s with column `C` matching `value`.
- `FindMByC(value) (*M, bool)` finds the non-deleted `M` with column `C` matching `value`, and whether it was found.
- `FindMByCUnscoped(value) (*M, bool)` finds the `M` with column `C` matching `value`, and whether it was found.

For an instance of model `M`:

- `Delete() bool` sets the `deleted_at` column to the current timestamp, and returns whether any rows were affected.
- `Restore() bool` sets the `deleted_at` column to `NULL`, and returns whether any rows were affected.
- `HardDelete() bool` deletes the row from the database, and whether it existed.
- `Reload()` fetches the latest values from the database.

For an instance of `M` and a column `C`:

- `UpdateC(value) bool` sets the value of column `C` to `value`.
- `ClearC() bool` sets the value of column `C` to `NULL`.

Helper methods for operations that are irrelevant for the type in question are not generated. For example, `Unscoped` variants won't be generated for models without a `deleted_at` column, and `ClearX` will only be generated for nullable columns.

## More features

Aside from the model-specific methods above, a number of more general helper methods are also available.

You can check out the package-level documentation for the specifics, but here is a quick overview, grouped by category.

- `Query`, `Exec`, and `QueryRow` query the database using raw SQL and return raw interfaces from `database/sql`.
- `Scan1` and `ScanN` query the database using raw SQL and scan the result(s) into a struct `T` or `[]T`, respectively.
- `NSlice`, `N`, and `N2Native` convert values between `T` and `sql.Null[T]`.
- `NTime`, `Now`, `ToTime` simplify operations on time.
- `Pluck`, `MapByID`, `MapBy`, `MapBy2` extract values out of structs into different datatypes like maps and slices.
- `BeginTransaction`, `CommitTransaction`, `RollbackTransaction` handle database transactions.

## Installation

There are two ways to use sqlc plugins: as a standard binary or a sandboxed wasm binary. For security, wasm binaries are recommended when running untrusted plugins, however, a significant performance penalty is incurred in this case. As this repository is a mere ~1100 lines of code it's recommended to vet the code and then use the faster, process-based method.

For completeness both are documented below. For more information on using plugins [refer to the sqlc documentation](https://docs.sqlc.dev/en/latest/guides/plugins.html).

### Process

Go to the [releases page](https://github.com/crdx/starfield/releases) and download the latest `starfield` binary.

Define it as a plugin in `sqlc.yml`:

```yaml
plugins:
  - name: starfield
    process:
      cmd: /path/to/starfield
```

### Wasm

Go to the [releases page](https://github.com/crdx/starfield/releases) and copy the URL for the latest `starfield.wasm` binary. Calculate its sha256 hash:

```bash
curl -fsSL https://github.com/crdx/starfield/releases/download/.../starfield.wasm | sha256sum -
```

Define it as a plugin in `sqlc.yml`:

```yaml
plugins:
  - name: starfield
    wasm:
      url: https://github.com/crdx/starfield/releases/download/.../starfield.wasm
      sha256: xxx
```

## Usage

### Init

See the template [sqlc.yml](https://github.com/crdx/starfield/blob/main/scaffold/sqlc.yml) for an example of how to use the plugin. Alternatively, use the `starfieldctl init` command to create a basic project structure in the current directory.

You will want to call the `Init` function from the generated package (which is `db` by default) to set up the database connection. For documentation on each member below refer to the package documentation.

```go
db.Init(&db.Config{
    Open: func(dsn *db.DSN) (*sql.DB, error) {
        return sql.Open("mysql", dsn.Format())
    },
    DataSource: db.NewDSN().Apply(func(dsn *db.DSN) *db.DSN {
        dsn.DBName = "foo"
        dsn.Username = "user"
        dsn.Password = "hunter2"
        dsn.Protocol = "tcp"
        dsn.Address = "127.0.0.1:3306"
        return dsn
    }),
    Migrations:   migrations.List(),
    Create:       true,
    EnableLogger: env.Debug(),
    // Fresh:        false
    // ErrorHandler: func(err error) { panic(err) },
    // Seed:         func() { },
})
```

For SQLite the DSN can be omitted and the call to `sql.Open` handled directly in the `Open` method.

```go
db.Init(&db.Config{
    Open: func(_ *db.DSN) (*sql.DB, error) {
        return sql.Open("sqlite", "db.sqlite")
    },
    // ...
}
```

### Migration generator

Use `starfieldctl make-migration <name>` to make a migration. The name will be converted to snake case, if needed.

## Contributions

Open an [issue](https://github.com/crdx/starfield/issues) or send a [pull request](https://github.com/crdx/starfield/pulls).

## Licence

[GPLv3](LICENCE).
