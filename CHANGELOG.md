# Changelog

## [1.13.0] - 2026-09-28

### Added

- Support for `sqlc.embed()`

### Changed

- Map `tinyint(1)` to bool only
- Map `json` column to `json.RawMessage`
- Refuse `:copyfrom` and `:batch` commands

### Fixed

- MySQL `enum` columns generating `any`
- A `:one` query column that clashes with a parameter name

## [1.12.0] - 2026-09-27

### Added

- `Find<Model>By<Field>Unscoped` finders that include soft-deleted rows
- Support for non-integer model IDs

### Fixed

- SQLite migrations failing on the MySQL-only migration lock
- Generated `Create` functions for tables with a nullable `created_at` column
- Generated code failing to compile when names clash with identifiers or keywords
- A failed `BeginTransaction` leaving the connection in a broken state
- Migration row iteration errors being silently ignored
- Plugin option defaults not being applied when no options are given
- `starfieldctl init` scaffold missing the `db` package import

## [1.11.1] - 2026-07-28

### Fixed

- Prevent migration race between multiple dbs
- Close the rows handle when we should

## [1.11.0] - 2026-07-27

### Added

- `--target` for `starfieldctl make-migration` to select which db to target

### Fixed

- `starfieldctl make-migration` failing when using more than one db

## [1.10.1] - 2026-06-30

### Added

- Support for `bigint signed` and `bigint unsigned` column types

## [1.10.0] - 2026-04-12

### Added

- Release artifact of a `*.tar.gz` file containing both binaries

### Fixed

- Types for unsigned ID columns

## [1.9.0] - 2025-12-11

### Changed

#### Migration timestamp format

The default migration filename format has changed from Unix timestamps to a human-readable datetime.

```
1733900000_add_users.sql      # before
20251211120000_add_users.sql  # now
```

The new format uses `YYYYMMDDHHMMSS` (14 digits) instead of Unix epoch seconds (10 digits). Use the `--unix` flag to generate migrations in the legacy format.

#### Generated function names

Functions that return multiple records now use grammatically correct plural forms instead of simply appending "s".

```go
FindPersons()       // before
FindPeople()        // now

FindPersonsByName() // before
FindPeopleByName()  // now
```

## [1.8.1] - 2025-10-04

### Changed

- Upgrade dependencies
- Set minimum go version to 1.25

## [1.8.0] - 2025-10-04

### Changed

- Read schema directory from sqlc config file when making a migration

## [1.7.0] - 2025-08-09

### Added

- `starfieldctl version` command

## [1.5.0] - 2025-08-09

### Added

- `starfieldctl` binary in the release

## [1.4.0] - 2025-08-09

### Added

- `starfieldctl make-migration` command

### Changed

- Split out binaries into `starfieldctl` and `starfield`
- Adjust `starfieldctl init` template defaults
- Upgrade dependencies

## [1.3.0] - 2025-06-07

### Fixed

- Truncate `db.Now` to the nearest microsecond so that when it's passed into a query it's not turned into the nanosecond version, which is not supported. This can have subtle side effects like preventing the use of certain indexes

## [1.2.0] - 2025-05-09

### Fixed

- `BeginTransaction` sometimes incorrectly returning an error

## [1.1.0] - 2025-04-06

### Changed

- Disable linting of generated code
- Upgrade to go 1.24

## [1.0.0] - 2025-03-20

Introducing v1.0.0 of starfield, released purely because it's been used in production for long enough now that I'm reasonably sure no major breaking changes will be needed. (Update: I was wrong.)

### Added

- For each table generate a method that returns an instance of `TableStatus`. This struct contains information about the table
- Place a copy of the query in the comment of the function so that it's considered part of the documentation. Editors can display it on hover over (for example)

### Fixed

- Casing of "UUID" in generated function names

## [0.3.1] - 2024-09-21

Nothing of note.

## [0.3.0] - 2024-08-24

### Added

- `N2Native` function

## [0.2.0] - 2024-03-30

### Changed

- Format more numeric types in query logging output

## [0.1.0] - 2024-03-29

### Added

- Initial release
