# Changelog

## [Next]

## [1.14.0] — 2026-10-21

- Add `DSN.InterpolateParams`, enabled by default for MySQL
- Add `limit 1` to single-row `Find<Model>By<Field>` finders

## [1.13.1] — 2026-10-06

- Trim whitespace from generated SQL

## [1.13.0] — 2026-09-28

- Add support for `sqlc.embed()`
- Map `tinyint(1)` to bool only
- Map `json` column to `json.RawMessage`
- Refuse `:copyfrom` and `:batch` commands
- Fix MySQL `enum` columns generating `any`
- Fix a `:one` query column that clashes with a parameter name

## [1.12.0] — 2026-09-27

- Add `Find<Model>By<Field>Unscoped` finders that include soft-deleted rows
- Add support for non-integer model IDs
- Fix SQLite migrations failing on the MySQL-only migration lock
- Fix generated `Create` functions for tables with a nullable `created_at` column
- Fix generated code failing to compile when names clash with identifiers or keywords
- Fix a failed `BeginTransaction` leaving the connection in a broken state
- Fix migration row iteration errors being silently ignored
- Fix plugin option defaults not being applied when no options are given
- Fix `starfieldctl init` scaffold missing the `db` package import

## [1.11.1] — 2026-07-28

- Prevent migration race between multiple dbs
- Close the rows handle when we should

## [1.11.0] — 2026-07-27

- Add `--target` to `starfieldctl make-migration` to select which db to target
- Fix `starfieldctl make-migration` failing when using more than one db

## [1.10.1] — 2026-06-30

- Add support for `bigint signed` and `bigint unsigned` column types

## [1.10.0] — 2026-04-12

- Add a release artifact of a `*.tar.gz` file containing both binaries
- Fix types for unsigned ID columns

## [1.9.0] — 2025-12-11

### Migration timestamp format

The default migration filename format has changed from Unix timestamps to a human-readable datetime.

```
1733900000_add_users.sql      # before
20251211120000_add_users.sql  # now
```

The new format uses `YYYYMMDDHHMMSS` (14 digits) instead of Unix epoch seconds (10 digits). Use the `--unix` flag to generate migrations in the legacy format.

### Generated function names

Functions that return multiple records now use grammatically correct plural forms instead of simply appending "s".

```go
FindPersons()       // before
FindPeople()        // now

FindPersonsByName() // before
FindPeopleByName()  // now
```

## [1.8.1] — 2025-10-04

- Upgrade dependencies
- Set minimum go version to 1.25

## [1.8.0] — 2025-10-04

- Read schema directory from sqlc config file when making a migration

## [1.7.0] — 2025-08-09

- Add `starfieldctl version` command

## [1.5.0] — 2025-08-09

- Add `starfieldctl` binary to the release

## [1.4.0] — 2025-08-09

- Add `starfieldctl make-migration` command
- Split out binaries into `starfieldctl` and `starfield`
- Adjust `starfieldctl init` template defaults
- Upgrade dependencies

## [1.3.0] — 2025-06-07

- Truncate `db.Now` to the nearest microsecond so that when it's passed into a query it's not turned into the nanosecond version, which is not supported. This can have subtle side effects like preventing the use of certain indexes

## [1.2.0] — 2025-05-09

- Fix `BeginTransaction` sometimes incorrectly returning an error

## [1.1.0] — 2025-04-06

- Disable linting of generated code
- Upgrade to go 1.24

## [1.0.0] — 2025-03-20

Introducing v1.0.0 of starfield, released purely because it's been used in production for long enough now that I'm reasonably sure no major breaking changes will be needed. (Update: I was wrong.)

- For each table generate a method that returns an instance of `TableStatus`. This struct contains information about the table
- Place a copy of the query in the comment of the function so that it's considered part of the documentation. Editors can display it on hover over (for example)
- Fix casing of "UUID" in generated function names

## [0.3.1] — 2024-09-21

Nothing of note.

## [0.3.0] — 2024-08-24

- Add `N2Native` function

## [0.2.0] — 2024-03-30

- Format more numeric types in query logging output

## [0.1.0] — 2024-03-29

- Initial release
