// Code generated with:
//
//	sqlc     	v1.31.1
//	starfield	v1.11.1
//
//nolint:all
package db

import (
	"bytes"
	"context"
	"database/sql"
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
	if oldConnection == nil {
		oldConnection = connection
		var err error
		connection, err = connection.(*sql.DB).Begin()
		return err
	}
	return errors.New("transaction already started")
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

type Counter struct {
	Name  string `column:"name"`
	Value int64  `column:"value"`
}

// GetCounterTableStatus returns the TableStatus for the Counters table.
func GetCounterTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "counters"`)

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

// CreateCounter creates a new instance of Counter.
func CreateCounter(value *Counter) *Counter {
	_, err := Exec("insert into `counters` (`name`, `value`) values (?, ?)",
		value.Name,
		value.Value,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	return value
}

// FindCounters finds all non-deleted instances of Counter.
func FindCounters() []*Counter {
	rows, err := Query("select `name`, `value` from `counters` order by `name` asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Counter
	for rows.Next() {
		item := new(Counter)
		if err := rows.Scan(
			&item.Name,
			&item.Value,
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

// FindCountersByName finds all non-deleted instances of Counter by Name.
func FindCountersByName(value string) []*Counter {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `name`, `value` from `counters` where `name` = ? order by `name` asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Counter
	for rows.Next() {
		item := new(Counter)
		if err := rows.Scan(
			&item.Name,
			&item.Value,
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

// FindCounterByName finds an instance of Counter by Name.
func FindCounterByName(value string) (*Counter, bool) {
	row := QueryRow("select `name`, `value` from `counters` where `name` = ?", value)
	item := new(Counter)
	err := row.Scan(
		&item.Name,
		&item.Value,
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

// FindCountersByValue finds all non-deleted instances of Counter by Value.
func FindCountersByValue(value int64) []*Counter {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `name`, `value` from `counters` where `value` = ? order by `name` asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Counter
	for rows.Next() {
		item := new(Counter)
		if err := rows.Scan(
			&item.Name,
			&item.Value,
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

// FindCounterByValue finds an instance of Counter by Value.
func FindCounterByValue(value int64) (*Counter, bool) {
	row := QueryRow("select `name`, `value` from `counters` where `value` = ?", value)
	item := new(Counter)
	err := row.Scan(
		&item.Name,
		&item.Value,
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

type Document struct {
	ID        string              `column:"id"`
	CreatedAt sql.Null[time.Time] `column:"created_at"`
	DeletedAt sql.Null[time.Time] `column:"deleted_at"`
	Body      string              `column:"body"`
	Payload   sql.Null[string]    `column:"payload"`
	Rank      float64             `column:"rank"`
	Active    bool                `column:"active"`
}

// GetDocumentTableStatus returns the TableStatus for the Documents table.
func GetDocumentTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "documents"`)

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

// CreateDocument creates a new instance of Document.
func CreateDocument(value *Document) *Document {
	if !value.CreatedAt.Valid || value.CreatedAt.V.IsZero() {
		value.CreatedAt = N(Now())
	}

	_, err := Exec("insert into `documents` (`id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active`) values (?, ?, ?, ?, ?, ?, ?)",
		value.ID,
		value.CreatedAt,
		value.DeletedAt,
		value.Body,
		value.Payload,
		value.Rank,
		value.Active,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	return value
}

// FindDocument finds a non-deleted instance of Document by ID.
func FindDocument[T Integer | ~string](id T) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where id = ? and deleted_at is null", id)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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

// FindDocumentUnscoped finds an instance (including deleted) of Document by ID.
func FindDocumentUnscoped[T Integer | ~string](id T) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where id = ?", id)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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

// FindDocuments finds all non-deleted instances of Document.
func FindDocuments() []*Document {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where deleted_at is null order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// Reload reloads this instance of Document.
func (self *Document) Reload() {
	if s, found := FindDocument(self.ID); found {
		*self = *s
	}
}

// FindDocumentsUnscoped finds all instances (including deleted) of Document.
func FindDocumentsUnscoped() []*Document {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// Delete soft deletes this instance of Document.
func (self *Document) Delete() bool {
	result, err := Exec("update `documents` set deleted_at = ? where id = ?", Now(), self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	return rowsAffected > 0
}

// Restore restores this instance of Document.
func (self *Document) Restore() bool {
	result, err := Exec("update `documents` set deleted_at = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	return rowsAffected > 0
}

// HardDelete hard deletes (i.e. DELETE) this instance of Document.
func (self *Document) HardDelete() bool {
	result, err := Exec("delete from `documents` where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	return rowsAffected > 0
}

// FindDocumentsByBody finds all non-deleted instances of Document by Body.
func FindDocumentsByBody(value string) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `body` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentsByBodyUnscoped finds all instances (including deleted) of Document by Body.
func FindDocumentsByBodyUnscoped(value string) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `body` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentByBody finds an instance of Document by Body.
func FindDocumentByBody(value string) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `body` = ? AND deleted_at IS NULL", value)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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

// UpdateBody updates the Body field.
func (self *Document) UpdateBody(value string) bool {
	result, err := Exec("update `documents` set `body` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Body = value
	return rowsAffected > 0
}

// FindDocumentsByPayload finds all non-deleted instances of Document by Payload.
func FindDocumentsByPayload(value sql.Null[string]) []*Document {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `payload` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `payload` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentsByPayloadUnscoped finds all instances (including deleted) of Document by Payload.
func FindDocumentsByPayloadUnscoped(value sql.Null[string]) []*Document {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `payload` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `payload` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentByPayload finds an instance of Document by Payload.
func FindDocumentByPayload(value string) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `payload` = ? AND deleted_at IS NULL", value)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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
func (self *Document) UpdatePayload(value string) bool {
	result, err := Exec("update `documents` set `payload` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Payload = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearPayload sets the Payload field to NULL.
func (self *Document) ClearPayload() bool {
	result, err := Exec("update `documents` set `payload` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Payload = sql.Null[string]{}
	return rowsAffected > 0
}

// FindDocumentsByRank finds all non-deleted instances of Document by Rank.
func FindDocumentsByRank(value float64) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `rank` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentsByRankUnscoped finds all instances (including deleted) of Document by Rank.
func FindDocumentsByRankUnscoped(value float64) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `rank` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentByRank finds an instance of Document by Rank.
func FindDocumentByRank(value float64) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `rank` = ? AND deleted_at IS NULL", value)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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

// UpdateRank updates the Rank field.
func (self *Document) UpdateRank(value float64) bool {
	result, err := Exec("update `documents` set `rank` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Rank = value
	return rowsAffected > 0
}

// FindDocumentsByActive finds all non-deleted instances of Document by Active.
func FindDocumentsByActive(value bool) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `active` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentsByActiveUnscoped finds all instances (including deleted) of Document by Active.
func FindDocumentsByActiveUnscoped(value bool) []*Document {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `active` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// FindDocumentByActive finds an instance of Document by Active.
func FindDocumentByActive(value bool) (*Document, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `body`, `payload`, `rank`, `active` from `documents` where `active` = ? AND deleted_at IS NULL", value)
	item := new(Document)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Body,
		&item.Payload,
		&item.Rank,
		&item.Active,
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

// UpdateActive updates the Active field.
func (self *Document) UpdateActive(value bool) bool {
	result, err := Exec("update `documents` set `active` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Active = value
	return rowsAffected > 0
}

type DocumentSummariesView struct {
	ID     string `column:"id"`
	Body   string `column:"body"`
	Active bool   `column:"active"`
}

// :exec
const deleteCounter = `
delete from counters where name = ?
`

// DeleteCounter runs an SQL query.
//
// delete from counters where name = ?
func DeleteCounter(name string) {
	_, err := Exec(deleteCounter, name)
	if err != nil {
		handleError(err)
	}
}

// :one
const findDocumentBody = `
select body from documents where id = ?
`

// FindDocumentBody runs an SQL query.
//
// select body from documents where id = ?
func FindDocumentBody(id string) (string, bool) {
	row := QueryRow(findDocumentBody, id)
	var body string
	err := row.Scan(&body)
	if err == sql.ErrNoRows {
		return body, false
	}
	if err != nil {
		handleError(err)
		return body, false
	}
	return body, true
}

// :many
const findDocumentsByIDs = `
select id, created_at, deleted_at, body, payload, rank, active from documents
where id in (/*SLICE:ids*/?)
order by id
`

// FindDocumentsByIDs runs an SQL query.
//
// select id, created_at, deleted_at, body, payload, rank, active from documents where id in (/*SLICE:ids*/?) order by id
func FindDocumentsByIDs(ids []string) []*Document {
	q := findDocumentsByIDs
	var queryParams []any
	if len(ids) > 0 {
		for _, v := range ids {
			queryParams = append(queryParams, v)
		}
		q = strings.Replace(q, "/*SLICE:ids*/?", strings.Repeat(",?", len(ids))[1:], 1)
	} else {
		q = strings.Replace(q, "/*SLICE:ids*/?", "NULL", 1)
	}

	rows, err := Query(q, queryParams...)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// :execresult
const incrementCounter = `
update counters set value = value + 1 where name = ?
`

// IncrementCounter runs an SQL query.
//
// update counters set value = value + 1 where name = ?
func IncrementCounter(name string) sql.Result {
	result, err := Exec(incrementCounter, name)
	if err != nil {
		handleError(err)
	}
	return result
}

// :many
const listDocuments = `
select id, created_at, deleted_at, body, payload, rank, active from documents where deleted_at is null order by id
`

// ListDocuments runs an SQL query.
//
// select id, created_at, deleted_at, body, payload, rank, active from documents where deleted_at is null order by id
func ListDocuments() []*Document {
	rows, err := Query(listDocuments)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Document
	for rows.Next() {
		item := new(Document)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Body,
			&item.Payload,
			&item.Rank,
			&item.Active,
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

// :execrows
const updateDocument = `
update documents
set body = ?1, rank = ?2, active = ?3
where id = ?4
`

type UpdateDocumentParams struct {
	Body   string
	Rank   float64
	Active bool
	ID     string
}

// UpdateDocument runs an SQL query.
//
// update documents set body = ?1, rank = ?2, active = ?3 where id = ?4
func UpdateDocument(params UpdateDocumentParams) int64 {
	result, err := Exec(updateDocument,
		params.Body,
		params.Rank,
		params.Active,
		params.ID,
	)
	if err != nil {
		handleError(err)
		return 0
	}
	if rowsAffected, err := result.RowsAffected(); err != nil {
		handleError(err)
		return 0
	} else {
		return rowsAffected
	}
}
