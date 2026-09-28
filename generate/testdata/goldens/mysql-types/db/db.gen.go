// Code generated with:
//
//	sqlc     	v1.31.1
//	starfield	v1.13.0
//
//nolint:all
package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type Connection interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

var (
	connection    Connection
	oldConnection Connection
	config        *Config
)

type Config struct {
	// A function that opens a database connection using a DSN. If the DataSource is omitted nil
	// will be passed to this method, which indicates that you want to handle the DSN yourself.
	Open func(*DSN) (*sql.DB, error)

	// The DSN. Omit this if you want to handle the DSN yourself in the Open method.
	DataSource *DSN

	Migrations   []*Migration // Migrations to run.
	Fresh        bool         // Drop and recreate the database (e.g. for tests). Implies Create.
	Create       bool         // Create the database if it does not exist.
	EnableLogger bool         // Log queries and their execution time to stderr.
	ErrorHandler func(error)  // Alternative error handler to panic().
	Seed         func()       // Database seeder which only runs if the database is freshly created.
}

// Init sets up the database.
func Init(c *Config) error {
	config = c
	return doInit()
}

func doInit() error {
	var baseDSN *DSN
	if config.DataSource != nil {
		// The base DSN has no database name.
		baseDSN = config.DataSource.Clone()
		baseDSN.DBName = ""
	}

	if config.Fresh {
		if _, err := dropDatabase(baseDSN, config.DataSource.DBName); err != nil {
			return err
		}
	}

	var seed bool
	if config.Create || config.Fresh {
		if created, err := createDatabase(baseDSN, config.DataSource.DBName); err != nil {
			return err
		} else if created {
			seed = true
		}
	}

	var migratorDSN *DSN
	if config.DataSource != nil {
		// The migrator DSN supports multiple statements so it can run migrations.
		migratorDSN = config.DataSource.Clone()
		migratorDSN.MultiStatements = true
	}

	if err := migrate(migratorDSN); err != nil {
		return err
	}

	var err error
	connection, err = config.Open(config.DataSource)

	if err == nil && seed && config.Seed != nil {
		config.Seed()
	}

	return err
}

// Query is the escape hatch that lets you call the underlying sql.DB.Query method.
func Query(query string, args ...any) (*sql.Rows, error) {
	if config.EnableLogger {
		t := time.Now()
		defer func() {
			logQuery(formatQuery(query, args...), time.Since(t))
		}()
	}
	return connection.Query(query, args...)
}

// Exec is the escape hatch that lets you call the underlying sql.DB.Exec method.
func Exec(query string, args ...any) (sql.Result, error) {
	if config.EnableLogger {
		t := time.Now()
		defer func() {
			logQuery(formatQuery(query, args...), time.Since(t))
		}()
	}
	return connection.Exec(query, args...)
}

// QueryRow is the escape hatch that lets you call the underlying sql.DB.QueryRow method.
func QueryRow(query string, args ...any) *sql.Row {
	if config.EnableLogger {
		t := time.Now()
		defer func() {
			logQuery(formatQuery(query, args...), time.Since(t))
		}()
	}
	return connection.QueryRow(query, args...)
}

// Scan1 scans one row into *T.
func Scan1[T any](query string, args ...any) *T {
	items := ScanN[T](query, args...)
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

// ScanN scans N rows into []*T.
func ScanN[T any](query string, args ...any) []*T {
	rows, err := Query(query, args...)
	if err != nil {
		handleError(err)
		return nil
	}

	defer rows.Close() //nolint:errcheck

	var items []*T
	for rows.Next() {
		item := new(T)

		if err := rows.Scan(getStructFields(item)...); err != nil {
			handleError(err)
			return nil
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}

	return items
}

// NSlice converts a []T into a []sql.Null[T], effectively wrapping each slice element in
// sql.Null[T].
func NSlice[T any](v []T) []sql.Null[T] {
	var items []sql.Null[T]
	for _, item := range v {
		items = append(items, N(item))
	}
	return items
}

// N converts a T into a sql.Null[T].
func N[T any](v T) sql.Null[T] {
	return sql.Null[T]{V: v, Valid: true}
}

// N2Native converts a sql.Null[T] into a *T. A null instance of sql.Null[T] is mapped to nil
// for *T.
func N2Native[T any](v sql.Null[T]) *T {
	if v.Valid {
		return &v.V
	} else {
		return nil
	}
}

// NTime parses a string into a valid sql.Null[time.Time]. If it cannot be parsed, it returns a null
// instance of sql.Null[time.Time].
func NTime(s string, layout string) sql.Null[time.Time] {
	var v sql.Null[time.Time]
	t, err := time.Parse(layout, s)
	if err != nil {
		return v
	}
	if s != "" {
		v.V = t
		v.Valid = true
	}
	return v
}

// Now returns the current time.Time in UTC.
//
// Truncated to the nearest microsecond so that when it's passed into a query it's not turned
// into the nanosecond version, which is not supported. This can have subtle side-effects like
// preventing the use of certain indexes.
func Now() time.Time {
	return time.Now().UTC().Round(time.Microsecond)
}

// ToTime returns the time.Time that corresponds to the passed value. The value must be either
// time.Time or sql.Null[time.Time].
func ToTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case sql.Null[time.Time]:
		if t.Valid {
			return t.V, true
		}
	case time.Time:
		return t, true
	}
	return time.Time{}, false
}

// Pluck extracts a slice of a single field from a slice of structs.
func Pluck[T any, U any](items []U, field string) []T {
	var values []T
	for _, item := range items {
		values = append(values, reflect.ValueOf(item).Elem().FieldByName(field).Interface().(T))
	}
	return values
}

// MapByID maps a slice of T to a map[int64]T using the ID field as the key. If T is not a struct,
// or the ID field does not exist in T, this function will panic.
func MapByID[T any](items []T) map[int64]T {
	return MapBy[int64]("ID", items)
}

// MapBy maps a slice of T to a map[A]T using keyField as the key. If T is not a struct, or
// keyField does not exist in T, this function will panic.
func MapBy[A comparable, T any](keyField string, items []T) map[A]T {
	return sliceToMap(items, func(item T) (A, T) {
		return reflect.ValueOf(item).Elem().FieldByName(keyField).Interface().(A), item
	})
}

// MapBy2 maps a slice of T to a map[A]B using keyField and valueField as the key and value. If T
// is not a struct, or keyField or valueField do not exist in T, this function will panic.
func MapBy2[A comparable, B any, T any](keyField, valueField string, items []T) map[A]B {
	return sliceToMap(items, func(item T) (A, B) {
		v := reflect.ValueOf(item).Elem()
		keyFieldValue := v.FieldByName(keyField)
		valueFieldValue := v.FieldByName(valueField)
		return convertValue[A](keyFieldValue),
			convertValue[B](valueFieldValue)
	})
}

type TableStatus struct {
	Name           string
	Engine         string
	Version        int
	RowFormat      string
	Rows           int
	AvgRowLength   int
	DataLength     int64
	MaxDataLength  int64
	IndexLength    int64
	DataFree       int64
	AutoIncrement  int
	CreateTime     string
	UpdateTime     *string
	CheckTime      *string
	Collation      string
	Checksum       *string
	CreateOptions  string
	Comment        string
	MaxIndexLength int64
	Temporary      string
}

type Migration struct {
	Name string
	SQL  string
}

const migrateLockTimeout = 30

func migrate(dsn *DSN) error {
	pool, err := config.Open(dsn)
	if err != nil {
		return err
	}
	defer pool.Close() //nolint:errcheck

	ctx := context.Background()
	connection, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close() //nolint:errcheck

	lockName := "sf_migrate_" + dsn.DBName
	if len(lockName) > 64 {
		lockName = lockName[:64]
	}

	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "select get_lock(?, ?)", lockName, migrateLockTimeout).Scan(&acquired); err != nil {
		return err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("timed out after %ds acquiring migration lock %s", migrateLockTimeout, lockName)
	}

	defer connection.ExecContext(ctx, "select release_lock(?)", lockName) //nolint:errcheck

	_, err = connection.ExecContext(ctx, `
			create table if not exists migrations (
				name varchar(512) not null primary key,
				migrated_at datetime not null
			)
		`)
	if err != nil {
		return err
	}

	rows, err := connection.QueryContext(ctx, "select name from migrations")
	if err != nil {
		return err
	}

	ran := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close() //nolint:errcheck
			return err
		}
		ran[id] = true
	}
	rows.Close() //nolint:errcheck
	if err := rows.Err(); err != nil {
		return err
	}

	for _, migration := range config.Migrations {
		if ran[migration.Name] {
			continue
		}

		sql := strings.TrimSpace(migration.SQL)

		if config.EnableLogger {
			if sql == "" {
				log.Printf("\033[33mSkipping empty migration %s\033[0m", migration.Name)
			} else {
				log.Printf("\033[33mRunning migration %s\033[0m", migration.Name)
			}
		}

		if sql == "" {
			continue
		}

		_, err = connection.ExecContext(ctx, sql)
		if err != nil {
			return fmt.Errorf("%s: %w", migration.Name, err)
		}

		_, err = connection.ExecContext(
			ctx,
			"insert into migrations (name, migrated_at) values (?, ?)",
			migration.Name,
			Now().UTC(),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// BeginTransaction starts a new transaction. If one has already been started, this function
// returns an error. All subsequent database calls will take place within this transaction,
// so this is NOT thread-safe.
func BeginTransaction() error {
	if oldConnection != nil {
		return errors.New("transaction already started")
	}

	transaction, err := connection.(*sql.DB).Begin()
	if err != nil {
		return err
	}

	oldConnection = connection
	connection = transaction
	return nil
}

// CommitTransaction commits the current transaction, if any.
func CommitTransaction() error {
	if oldConnection == nil {
		return errors.New("no transaction")
	}
	err := connection.(*sql.Tx).Commit()
	connection = oldConnection
	oldConnection = nil
	if err != nil {
		return err
	}
	return nil
}

// RollbackTransaction rolls back the current transaction, if any.
func RollbackTransaction() error {
	if oldConnection == nil {
		return errors.New("no transaction")
	}
	err := connection.(*sql.Tx).Rollback()
	connection = oldConnection
	oldConnection = nil
	return err
}

func createDatabase(dsn *DSN, dbName string) (bool, error) {
	connection, err := config.Open(dsn)
	if err != nil {
		return false, err
	}
	defer connection.Close() //nolint:errcheck

	result, err := connection.Exec("CREATE DATABASE IF NOT EXISTS " + dbName)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

func dropDatabase(dsn *DSN, dbName string) (bool, error) {
	connection, err := config.Open(dsn)
	if err != nil {
		return false, err
	}
	defer connection.Close() //nolint:errcheck

	result, err := connection.Exec("DROP DATABASE IF EXISTS " + dbName)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

type DSN struct {
	Username             string         // Username
	Password             string         // Password
	Protocol             string         // Protocol (tcp, tcp6, unix)
	Address              string         // Address
	DBName               string         // Database name
	Charset              string         // Character set
	Timezone             *time.Location // Timezone for time.Time values [default: time.UTC]
	ParseTime            bool           // Parse time values to time.Time
	MultiStatements      bool           // Allow multiple statements in one query
	AllowNativePasswords bool           // Allow native password authentication
}

func NewDSN() *DSN {
	return &DSN{
		Timezone:             time.UTC,
		ParseTime:            true,
		AllowNativePasswords: true,
	}
}

func (self *DSN) Clone() *DSN {
	clone := *self
	return &clone
}

// Apply applies the given functions to the current DSN object.
func (self *DSN) Apply(funcs ...func(*DSN) *DSN) *DSN {
	for _, f := range funcs {
		f(self)
	}
	return self
}

// Format formats the current DSN into a string.
func (self *DSN) Format() string {
	var buf bytes.Buffer

	// [username[:password]@]
	if len(self.Username) > 0 {
		buf.WriteString(self.Username)
		if len(self.Password) > 0 {
			buf.WriteByte(':')
			buf.WriteString(self.Password)
		}
		buf.WriteByte('@')
	}

	// [protocol[(address)]]
	if len(self.Protocol) > 0 {
		buf.WriteString(self.Protocol)
		if len(self.Address) > 0 {
			buf.WriteByte('(')
			buf.WriteString(self.Address)
			buf.WriteByte(')')
		}
	}

	// /dbname
	buf.WriteByte('/')
	buf.WriteString(url.PathEscape(self.DBName))

	// [?param1=value1&...&paramN=valueN]
	hasParam := false

	if !self.AllowNativePasswords {
		writeDSNParam(&buf, &hasParam, "allowNativePasswords", "false")
	}

	if self.Timezone != time.UTC && self.Timezone != nil {
		writeDSNParam(&buf, &hasParam, "loc", url.QueryEscape(self.Timezone.String()))
	}

	if self.Charset != "" {
		writeDSNParam(&buf, &hasParam, "charset", self.Charset)
	}

	if self.MultiStatements {
		writeDSNParam(&buf, &hasParam, "multiStatements", "true")
	}

	if self.ParseTime {
		writeDSNParam(&buf, &hasParam, "parseTime", "true")
	}

	return buf.String()
}

func writeDSNParam(buf *bytes.Buffer, hasParam *bool, name, value string) {
	buf.Grow(1 + len(name) + 1 + len(value))
	if !*hasParam {
		*hasParam = true
		buf.WriteByte('?')
	} else {
		buf.WriteByte('&')
	}
	buf.WriteString(name)
	buf.WriteByte('=')
	buf.WriteString(value)
}

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type Integer interface {
	Signed | Unsigned
}

func logQuery(s string, d time.Duration) {
	file, line := getCaller(5)
	link := hyperlinkFile(fmt.Sprintf("%s#%d", file, line), fmt.Sprintf("%s:%d", filepath.Base(file), line))
	log.Printf("[\033[33m%3dms\033[0m] \033[36m%s\033[0m %s", d.Truncate(time.Millisecond).Milliseconds(), link, s)
}

func formatQuery(query string, args ...any) string {
	for _, arg := range args {
		var s string
		switch a := arg.(type) {
		case bool:
			if a {
				s = "1"
			} else {
				s = "0"
			}
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
			s = fmt.Sprintf("%v", a)
		case sql.Null[time.Time]:
			if a.Valid {
				s = fmt.Sprintf("%q", a.V.Format(time.DateTime))
			} else {
				s = "NULL"
			}
		case sql.Null[int64]:
			if a.Valid {
				s = fmt.Sprintf("%v", a.V)
			} else {
				s = "NULL"
			}
		case sql.Null[float64]:
			if a.Valid {
				s = fmt.Sprintf("%v", a.V)
			} else {
				s = "NULL"
			}
		case time.Time:
			s = fmt.Sprintf("%q", a.Format(time.DateTime))
		default:
			s = fmt.Sprintf("%q", a)
		}
		query = strings.Replace(query, "?", s, 1)
	}
	query = strings.ReplaceAll(strings.TrimSpace(query), "\n", " ")
	query = strings.ReplaceAll(query, "`", "")
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")
	return query
}

func handleError(err error) {
	if config.ErrorHandler != nil {
		config.ErrorHandler(err)
	} else {
		panic(err)
	}
}

// canBeNil checks if the reflect.Value can be nil based on its kind.
func canBeNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Interface, reflect.Func:
		return true
	default:
		return false
	}
}

// zeroValue returns a pointer to a new zero value of type T.
func zeroValue[T any]() T {
	var zero T
	return zero
}

// SliceToMap returns a map containing key-value pairs provided by transform function applied to
// elements of the given slice. If any of two pairs would have the same key the last one gets added
// to the map. The order of keys in returned map is not specified and is not guaranteed to be the
// same from the original array.
func sliceToMap[T any, K comparable, V any](collection []T, transform func(item T) (K, V)) map[K]V {
	result := make(map[K]V, len(collection))

	for _, t := range collection {
		k, v := transform(t)
		result[k] = v
	}

	return result
}

// convertValue converts a reflect.Value into T taking into account nil.
func convertValue[T any](v reflect.Value) T {
	if canBeNil(v) && v.IsNil() {
		return zeroValue[T]()
	} else {
		return v.Interface().(T)
	}
}

func getStructFields(v any) []any {
	value := reflect.ValueOf(v).Elem()
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	fields := make([]any, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		fields[i] = value.Field(i).Addr().Interface()
	}

	return fields
}

func getCaller(level int) (string, int) {
	_, file, line, ok := runtime.Caller(level)
	if !ok {
		return "", 0
	}

	return file, line
}

func hyperlinkFile(link string, text string) string {
	return fmt.Sprintf("\033]8;;file://%s\033\\%s\033]8;;\033\\", link, text)
}

type Thing struct {
	ID           uint64                    `column:"id"`
	Name         string                    `column:"name"`
	Flag         bool                      `column:"flag"`
	MaybeFlag    sql.Null[bool]            `column:"maybe_flag"`
	Rating       uint64                    `column:"rating"`
	OffsetValue  sql.Null[int64]           `column:"offset_value"`
	Payload      json.RawMessage           `column:"payload"`
	MaybePayload sql.Null[json.RawMessage] `column:"maybe_payload"`
	Kind         string                    `column:"kind"`
	MaybeKind    sql.Null[string]          `column:"maybe_kind"`
}

// GetThingTableStatus returns the TableStatus for the Things table.
func GetThingTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "things"`)

	var status TableStatus
	err := row.Scan(
		&status.Name,
		&status.Engine,
		&status.Version,
		&status.RowFormat,
		&status.Rows,
		&status.AvgRowLength,
		&status.DataLength,
		&status.MaxDataLength,
		&status.IndexLength,
		&status.DataFree,
		&status.AutoIncrement,
		&status.CreateTime,
		&status.UpdateTime,
		&status.CheckTime,
		&status.Collation,
		&status.Checksum,
		&status.CreateOptions,
		&status.Comment,
		&status.MaxIndexLength,
		&status.Temporary,
	)

	if err != nil {
		return TableStatus{}, err
	}

	return status, nil
}

// CreateThing creates a new instance of Thing.
func CreateThing(value *Thing) *Thing {
	result, err := Exec("insert into `things` (`id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind`) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		value.ID,
		value.Name,
		value.Flag,
		value.MaybeFlag,
		value.Rating,
		value.OffsetValue,
		value.Payload,
		value.MaybePayload,
		value.Kind,
		value.MaybeKind,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	if value.ID == 0 {
		lastInsertID, err := result.LastInsertId()
		if err != nil {
			handleError(err)
			return nil
		}
		value.ID = uint64(lastInsertID)
	}

	return value
}

// FindThing finds a non-deleted instance of Thing by ID.
func FindThing[T Integer | ~string](id T) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where id = ?", id)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return nil, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// FindThingUnscoped finds an instance (including deleted) of Thing by ID.
func FindThingUnscoped[T Integer | ~string](id T) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where id = ?", id)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return nil, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// FindThings finds all non-deleted instances of Thing.
func FindThings() []*Thing {
	rows, err := Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// Reload reloads this instance of Thing.
func (self *Thing) Reload() {
	if s, found := FindThing(self.ID); found {
		*self = *s
	}
}

// FindThingsByName finds all non-deleted instances of Thing by Name.
func FindThingsByName(value string) []*Thing {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `name` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByName finds an instance of Thing by Name.
func FindThingByName(value string) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `name` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateName updates the Name field.
func (self *Thing) UpdateName(value string) bool {
	result, err := Exec("update `things` set `name` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Name = value
	return rowsAffected > 0
}

// FindThingsByFlag finds all non-deleted instances of Thing by Flag.
func FindThingsByFlag(value bool) []*Thing {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `flag` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByFlag finds an instance of Thing by Flag.
func FindThingByFlag(value bool) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `flag` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateFlag updates the Flag field.
func (self *Thing) UpdateFlag(value bool) bool {
	result, err := Exec("update `things` set `flag` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Flag = value
	return rowsAffected > 0
}

// FindThingsByMaybeFlag finds all non-deleted instances of Thing by MaybeFlag.
func FindThingsByMaybeFlag(value sql.Null[bool]) []*Thing {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_flag` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_flag` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByMaybeFlag finds an instance of Thing by MaybeFlag.
func FindThingByMaybeFlag(value bool) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_flag` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateMaybeFlag updates the MaybeFlag field.
func (self *Thing) UpdateMaybeFlag(value bool) bool {
	result, err := Exec("update `things` set `maybe_flag` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybeFlag = sql.Null[bool]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearMaybeFlag sets the MaybeFlag field to NULL.
func (self *Thing) ClearMaybeFlag() bool {
	result, err := Exec("update `things` set `maybe_flag` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybeFlag = sql.Null[bool]{}
	return rowsAffected > 0
}

// FindThingsByRating finds all non-deleted instances of Thing by Rating.
func FindThingsByRating(value uint64) []*Thing {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `rating` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByRating finds an instance of Thing by Rating.
func FindThingByRating(value uint64) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `rating` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateRating updates the Rating field.
func (self *Thing) UpdateRating(value uint64) bool {
	result, err := Exec("update `things` set `rating` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Rating = value
	return rowsAffected > 0
}

// FindThingsByOffsetValue finds all non-deleted instances of Thing by OffsetValue.
func FindThingsByOffsetValue(value sql.Null[int64]) []*Thing {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `offset_value` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `offset_value` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByOffsetValue finds an instance of Thing by OffsetValue.
func FindThingByOffsetValue(value int64) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `offset_value` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateOffsetValue updates the OffsetValue field.
func (self *Thing) UpdateOffsetValue(value int64) bool {
	result, err := Exec("update `things` set `offset_value` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.OffsetValue = sql.Null[int64]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearOffsetValue sets the OffsetValue field to NULL.
func (self *Thing) ClearOffsetValue() bool {
	result, err := Exec("update `things` set `offset_value` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.OffsetValue = sql.Null[int64]{}
	return rowsAffected > 0
}

// FindThingsByPayload finds all non-deleted instances of Thing by Payload.
func FindThingsByPayload(value json.RawMessage) []*Thing {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `payload` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByPayload finds an instance of Thing by Payload.
func FindThingByPayload(value json.RawMessage) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `payload` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdatePayload updates the Payload field.
func (self *Thing) UpdatePayload(value json.RawMessage) bool {
	result, err := Exec("update `things` set `payload` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Payload = value
	return rowsAffected > 0
}

// FindThingsByMaybePayload finds all non-deleted instances of Thing by MaybePayload.
func FindThingsByMaybePayload(value sql.Null[json.RawMessage]) []*Thing {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_payload` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_payload` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByMaybePayload finds an instance of Thing by MaybePayload.
func FindThingByMaybePayload(value json.RawMessage) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_payload` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateMaybePayload updates the MaybePayload field.
func (self *Thing) UpdateMaybePayload(value json.RawMessage) bool {
	result, err := Exec("update `things` set `maybe_payload` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybePayload = sql.Null[json.RawMessage]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearMaybePayload sets the MaybePayload field to NULL.
func (self *Thing) ClearMaybePayload() bool {
	result, err := Exec("update `things` set `maybe_payload` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybePayload = sql.Null[json.RawMessage]{}
	return rowsAffected > 0
}

// FindThingsByKind finds all non-deleted instances of Thing by Kind.
func FindThingsByKind(value string) []*Thing {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `kind` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByKind finds an instance of Thing by Kind.
func FindThingByKind(value string) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `kind` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateKind updates the Kind field.
func (self *Thing) UpdateKind(value string) bool {
	result, err := Exec("update `things` set `kind` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Kind = value
	return rowsAffected > 0
}

// FindThingsByMaybeKind finds all non-deleted instances of Thing by MaybeKind.
func FindThingsByMaybeKind(value sql.Null[string]) []*Thing {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_kind` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_kind` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Thing
	for rows.Next() {
		item := new(Thing)
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Flag,
			&item.MaybeFlag,
			&item.Rating,
			&item.OffsetValue,
			&item.Payload,
			&item.MaybePayload,
			&item.Kind,
			&item.MaybeKind,
		); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// FindThingByMaybeKind finds an instance of Thing by MaybeKind.
func FindThingByMaybeKind(value string) (*Thing, bool) {
	row := QueryRow("select `id`, `name`, `flag`, `maybe_flag`, `rating`, `offset_value`, `payload`, `maybe_payload`, `kind`, `maybe_kind` from `things` where `maybe_kind` = ?", value)
	item := new(Thing)
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Flag,
		&item.MaybeFlag,
		&item.Rating,
		&item.OffsetValue,
		&item.Payload,
		&item.MaybePayload,
		&item.Kind,
		&item.MaybeKind,
	)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return nil, false
	}
	return item, true
}

// UpdateMaybeKind updates the MaybeKind field.
func (self *Thing) UpdateMaybeKind(value string) bool {
	result, err := Exec("update `things` set `maybe_kind` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybeKind = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearMaybeKind sets the MaybeKind field to NULL.
func (self *Thing) ClearMaybeKind() bool {
	result, err := Exec("update `things` set `maybe_kind` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.MaybeKind = sql.Null[string]{}
	return rowsAffected > 0
}

// :one
const countLarge = `
select count(*) from things where kind = 'large' and rating > ?
`

// CountLarge runs an SQL query.
//
// select count(*) from things where kind = 'large' and rating > ?
func CountLarge(rating uint64) int64 {
	row := QueryRow(countLarge, rating)
	var count int64
	err := row.Scan(&count)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		handleError(err)
		return 0
	}
	return count
}

// :many
const findKinds = `
select kind from things where maybe_kind = ?
`

// FindKinds runs an SQL query.
//
// select kind from things where maybe_kind = ?
func FindKinds(maybeKind sql.Null[string]) []string {
	rows, err := Query(findKinds, maybeKind)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, kind)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// :one
const findName = `
select name from things where name = ?
`

// FindName runs an SQL query.
// FindName returns a column named like its parameter, which the generated function must not
// declare twice.
//
// select name from things where name = ?
func FindName(name string) (string, bool) {
	row := QueryRow(findName, name)
	var name_2 string
	err := row.Scan(&name_2)
	if err == sql.ErrNoRows {
		return name_2, false
	}
	if err != nil {
		handleError(err)
		return name_2, false
	}
	return name_2, true
}

// :many
const findNames = `
select name from things where name like ?
`

// FindNames runs an SQL query.
//
// select name from things where name like ?
func FindNames(name string) []string {
	rows, err := Query(findNames, name)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, name)
	}
	if err := rows.Close(); err != nil {
		handleError(err)
		return nil
	}
	if err := rows.Err(); err != nil {
		handleError(err)
		return nil
	}
	return items
}

// :one
const findPayload = `
select payload from things where id = ?
`

// FindPayload runs an SQL query.
//
// select payload from things where id = ?
func FindPayload(id uint64) (json.RawMessage, bool) {
	row := QueryRow(findPayload, id)
	var payload json.RawMessage
	err := row.Scan(&payload)
	if err == sql.ErrNoRows {
		return payload, false
	}
	if err != nil {
		handleError(err)
		return payload, false
	}
	return payload, true
}
