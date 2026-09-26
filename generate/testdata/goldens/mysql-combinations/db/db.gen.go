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

type AuditEntry struct {
	ID          int64               `column:"id"`
	OccurredOn  time.Time           `column:"occurred_on"`
	Duration    sql.Null[time.Time] `column:"duration"`
	YearValue   int64               `column:"year_value"`
	Description string              `column:"description"`
}

// GetAuditEntryTableStatus returns the TableStatus for the AuditEntries table.
func GetAuditEntryTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "audit_entries"`)

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

// CreateAuditEntry creates a new instance of AuditEntry.
func CreateAuditEntry(value *AuditEntry) *AuditEntry {
	result, err := Exec("insert into `audit_entries` (`id`, `occurred_on`, `duration`, `year_value`, `description`) values (?, ?, ?, ?, ?)",
		value.ID,
		value.OccurredOn,
		value.Duration,
		value.YearValue,
		value.Description,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	lastInsertID, err := result.LastInsertId()
	if err != nil {
		handleError(err)
		return nil
	}
	value.ID = lastInsertID

	return value
}

// FindAuditEntry finds a non-deleted instance of AuditEntry by ID.
func FindAuditEntry[T Integer | ~string](id T) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where id = ?", id)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// FindAuditEntryUnscoped finds an instance (including deleted) of AuditEntry by ID.
func FindAuditEntryUnscoped[T Integer | ~string](id T) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where id = ?", id)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// FindAuditEntries finds all non-deleted instances of AuditEntry.
func FindAuditEntries() []*AuditEntry {
	rows, err := Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*AuditEntry
	for rows.Next() {
		item := new(AuditEntry)
		if err := rows.Scan(
			&item.ID,
			&item.OccurredOn,
			&item.Duration,
			&item.YearValue,
			&item.Description,
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

// Reload reloads this instance of AuditEntry.
func (self *AuditEntry) Reload() {
	if s, found := FindAuditEntry(self.ID); found {
		*self = *s
	}
}

// FindAuditEntriesByOccurredOn finds all non-deleted instances of AuditEntry by OccurredOn.
func FindAuditEntriesByOccurredOn(value time.Time) []*AuditEntry {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `occurred_on` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*AuditEntry
	for rows.Next() {
		item := new(AuditEntry)
		if err := rows.Scan(
			&item.ID,
			&item.OccurredOn,
			&item.Duration,
			&item.YearValue,
			&item.Description,
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

// FindAuditEntryByOccurredOn finds an instance of AuditEntry by OccurredOn.
func FindAuditEntryByOccurredOn(value time.Time) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `occurred_on` = ?", value)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// UpdateOccurredOn updates the OccurredOn field.
func (self *AuditEntry) UpdateOccurredOn(value time.Time) bool {
	result, err := Exec("update `audit_entries` set `occurred_on` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.OccurredOn = value
	return rowsAffected > 0
}

// FindAuditEntriesByDuration finds all non-deleted instances of AuditEntry by Duration.
func FindAuditEntriesByDuration(value sql.Null[time.Time]) []*AuditEntry {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `duration` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `duration` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*AuditEntry
	for rows.Next() {
		item := new(AuditEntry)
		if err := rows.Scan(
			&item.ID,
			&item.OccurredOn,
			&item.Duration,
			&item.YearValue,
			&item.Description,
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

// FindAuditEntryByDuration finds an instance of AuditEntry by Duration.
func FindAuditEntryByDuration(value time.Time) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `duration` = ?", value)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// UpdateDuration updates the Duration field.
func (self *AuditEntry) UpdateDuration(value time.Time) bool {
	result, err := Exec("update `audit_entries` set `duration` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Duration = sql.Null[time.Time]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearDuration sets the Duration field to NULL.
func (self *AuditEntry) ClearDuration() bool {
	result, err := Exec("update `audit_entries` set `duration` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Duration = sql.Null[time.Time]{}
	return rowsAffected > 0
}

// FindAuditEntriesByYearValue finds all non-deleted instances of AuditEntry by YearValue.
func FindAuditEntriesByYearValue(value int64) []*AuditEntry {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `year_value` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*AuditEntry
	for rows.Next() {
		item := new(AuditEntry)
		if err := rows.Scan(
			&item.ID,
			&item.OccurredOn,
			&item.Duration,
			&item.YearValue,
			&item.Description,
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

// FindAuditEntryByYearValue finds an instance of AuditEntry by YearValue.
func FindAuditEntryByYearValue(value int64) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `year_value` = ?", value)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// UpdateYearValue updates the YearValue field.
func (self *AuditEntry) UpdateYearValue(value int64) bool {
	result, err := Exec("update `audit_entries` set `year_value` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.YearValue = value
	return rowsAffected > 0
}

// FindAuditEntriesByDescription finds all non-deleted instances of AuditEntry by Description.
func FindAuditEntriesByDescription(value string) []*AuditEntry {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `description` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*AuditEntry
	for rows.Next() {
		item := new(AuditEntry)
		if err := rows.Scan(
			&item.ID,
			&item.OccurredOn,
			&item.Duration,
			&item.YearValue,
			&item.Description,
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

// FindAuditEntryByDescription finds an instance of AuditEntry by Description.
func FindAuditEntryByDescription(value string) (*AuditEntry, bool) {
	row := QueryRow("select `id`, `occurred_on`, `duration`, `year_value`, `description` from `audit_entries` where `description` = ?", value)
	item := new(AuditEntry)
	err := row.Scan(
		&item.ID,
		&item.OccurredOn,
		&item.Duration,
		&item.YearValue,
		&item.Description,
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

// UpdateDescription updates the Description field.
func (self *AuditEntry) UpdateDescription(value string) bool {
	result, err := Exec("update `audit_entries` set `description` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Description = value
	return rowsAffected > 0
}

type Metadata struct {
	Code  string           `column:"code"`
	Value sql.Null[string] `column:"value"`
}

// GetMetadataTableStatus returns the TableStatus for the MetadataList table.
func GetMetadataTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "metadata"`)

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

// CreateMetadata creates a new instance of Metadata.
func CreateMetadata(value *Metadata) *Metadata {
	_, err := Exec("insert into `metadata` (`code`, `value`) values (?, ?)",
		value.Code,
		value.Value,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	return value
}

// FindMetadataList finds all non-deleted instances of Metadata.
func FindMetadataList() []*Metadata {
	rows, err := Query("select `code`, `value` from `metadata` order by `code` asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Metadata
	for rows.Next() {
		item := new(Metadata)
		if err := rows.Scan(
			&item.Code,
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

// FindMetadataListByCode finds all non-deleted instances of Metadata by Code.
func FindMetadataListByCode(value string) []*Metadata {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `code`, `value` from `metadata` where `code` = ? order by `code` asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Metadata
	for rows.Next() {
		item := new(Metadata)
		if err := rows.Scan(
			&item.Code,
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

// FindMetadataByCode finds an instance of Metadata by Code.
func FindMetadataByCode(value string) (*Metadata, bool) {
	row := QueryRow("select `code`, `value` from `metadata` where `code` = ?", value)
	item := new(Metadata)
	err := row.Scan(
		&item.Code,
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

// FindMetadataListByValue finds all non-deleted instances of Metadata by Value.
func FindMetadataListByValue(value sql.Null[string]) []*Metadata {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `code`, `value` from `metadata` where `value` = ? order by `code` asc", value)
	} else {
		rows, err = Query("select `code`, `value` from `metadata` where `value` is null order by `code` asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Metadata
	for rows.Next() {
		item := new(Metadata)
		if err := rows.Scan(
			&item.Code,
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

// FindMetadataByValue finds an instance of Metadata by Value.
func FindMetadataByValue(value string) (*Metadata, bool) {
	row := QueryRow("select `code`, `value` from `metadata` where `value` = ?", value)
	item := new(Metadata)
	err := row.Scan(
		&item.Code,
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

type Resource struct {
	ID              int64               `column:"id"`
	CreatedAt       time.Time           `column:"created_at"`
	DeletedAt       sql.Null[time.Time] `column:"deleted_at"`
	APIURL          string              `column:"api_url"`
	Payload         []byte              `column:"payload"`
	OptionalPayload sql.Null[string]    `column:"optional_payload"`
	Ratio           float64             `column:"ratio"`
	Enabled         bool                `column:"enabled"`
	Category        any                 `column:"category"`
}

// GetResourceTableStatus returns the TableStatus for the Resources table.
func GetResourceTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "resources"`)

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

// CreateResource creates a new instance of Resource.
func CreateResource(value *Resource) *Resource {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = Now()
	}
	result, err := Exec("insert into `resources` (`id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category`) values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		value.ID,
		value.CreatedAt,
		value.DeletedAt,
		value.APIURL,
		value.Payload,
		value.OptionalPayload,
		value.Ratio,
		value.Enabled,
		value.Category,
	)
	if err != nil {
		handleError(err)
		return nil
	}

	lastInsertID, err := result.LastInsertId()
	if err != nil {
		handleError(err)
		return nil
	}
	value.ID = lastInsertID

	return value
}

// FindResource finds a non-deleted instance of Resource by ID.
func FindResource[T Integer | ~string](id T) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where id = ? and deleted_at is null", id)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// FindResourceUnscoped finds an instance (including deleted) of Resource by ID.
func FindResourceUnscoped[T Integer | ~string](id T) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where id = ?", id)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// FindResources finds all non-deleted instances of Resource.
func FindResources() []*Resource {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where deleted_at is null order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// Reload reloads this instance of Resource.
func (self *Resource) Reload() {
	if s, found := FindResource(self.ID); found {
		*self = *s
	}
}

// FindResourcesUnscoped finds all instances (including deleted) of Resource.
func FindResourcesUnscoped() []*Resource {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// Delete soft deletes this instance of Resource.
func (self *Resource) Delete() bool {
	result, err := Exec("update `resources` set deleted_at = ? where id = ?", Now(), self.ID)
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

// Restore restores this instance of Resource.
func (self *Resource) Restore() bool {
	result, err := Exec("update `resources` set deleted_at = null where id = ?", self.ID)
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

// HardDelete hard deletes (i.e. DELETE) this instance of Resource.
func (self *Resource) HardDelete() bool {
	result, err := Exec("delete from `resources` where id = ?", self.ID)
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

// FindResourcesByAPIURL finds all non-deleted instances of Resource by APIURL.
func FindResourcesByAPIURL(value string) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `api_url` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByAPIURLUnscoped finds all instances (including deleted) of Resource by APIURL.
func FindResourcesByAPIURLUnscoped(value string) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `api_url` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByAPIURL finds an instance of Resource by APIURL.
func FindResourceByAPIURL(value string) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `api_url` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// UpdateAPIURL updates the APIURL field.
func (self *Resource) UpdateAPIURL(value string) bool {
	result, err := Exec("update `resources` set `api_url` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.APIURL = value
	return rowsAffected > 0
}

// FindResourcesByPayload finds all non-deleted instances of Resource by Payload.
func FindResourcesByPayload(value []byte) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `payload` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByPayloadUnscoped finds all instances (including deleted) of Resource by Payload.
func FindResourcesByPayloadUnscoped(value []byte) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `payload` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByPayload finds an instance of Resource by Payload.
func FindResourceByPayload(value []byte) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `payload` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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
func (self *Resource) UpdatePayload(value []byte) bool {
	result, err := Exec("update `resources` set `payload` = ? where id = ?", value, self.ID)
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

// FindResourcesByOptionalPayload finds all non-deleted instances of Resource by OptionalPayload.
func FindResourcesByOptionalPayload(value sql.Null[string]) []*Resource {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `optional_payload` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `optional_payload` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByOptionalPayloadUnscoped finds all instances (including deleted) of Resource by OptionalPayload.
func FindResourcesByOptionalPayloadUnscoped(value sql.Null[string]) []*Resource {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `optional_payload` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `optional_payload` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByOptionalPayload finds an instance of Resource by OptionalPayload.
func FindResourceByOptionalPayload(value string) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `optional_payload` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// UpdateOptionalPayload updates the OptionalPayload field.
func (self *Resource) UpdateOptionalPayload(value string) bool {
	result, err := Exec("update `resources` set `optional_payload` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.OptionalPayload = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearOptionalPayload sets the OptionalPayload field to NULL.
func (self *Resource) ClearOptionalPayload() bool {
	result, err := Exec("update `resources` set `optional_payload` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.OptionalPayload = sql.Null[string]{}
	return rowsAffected > 0
}

// FindResourcesByRatio finds all non-deleted instances of Resource by Ratio.
func FindResourcesByRatio(value float64) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `ratio` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByRatioUnscoped finds all instances (including deleted) of Resource by Ratio.
func FindResourcesByRatioUnscoped(value float64) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `ratio` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByRatio finds an instance of Resource by Ratio.
func FindResourceByRatio(value float64) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `ratio` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// UpdateRatio updates the Ratio field.
func (self *Resource) UpdateRatio(value float64) bool {
	result, err := Exec("update `resources` set `ratio` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Ratio = value
	return rowsAffected > 0
}

// FindResourcesByEnabled finds all non-deleted instances of Resource by Enabled.
func FindResourcesByEnabled(value bool) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `enabled` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByEnabledUnscoped finds all instances (including deleted) of Resource by Enabled.
func FindResourcesByEnabledUnscoped(value bool) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `enabled` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByEnabled finds an instance of Resource by Enabled.
func FindResourceByEnabled(value bool) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `enabled` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// UpdateEnabled updates the Enabled field.
func (self *Resource) UpdateEnabled(value bool) bool {
	result, err := Exec("update `resources` set `enabled` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Enabled = value
	return rowsAffected > 0
}

// FindResourcesByCategory finds all non-deleted instances of Resource by Category.
func FindResourcesByCategory(value any) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `category` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourcesByCategoryUnscoped finds all instances (including deleted) of Resource by Category.
func FindResourcesByCategoryUnscoped(value any) []*Resource {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `category` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// FindResourceByCategory finds an instance of Resource by Category.
func FindResourceByCategory(value any) (*Resource, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `api_url`, `payload`, `optional_payload`, `ratio`, `enabled`, `category` from `resources` where `category` = ? AND deleted_at IS NULL", value)
	item := new(Resource)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.APIURL,
		&item.Payload,
		&item.OptionalPayload,
		&item.Ratio,
		&item.Enabled,
		&item.Category,
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

// UpdateCategory updates the Category field.
func (self *Resource) UpdateCategory(value any) bool {
	result, err := Exec("update `resources` set `category` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Category = value
	return rowsAffected > 0
}

type ResourcesView struct {
	ID      int64  `column:"id"`
	APIURL  string `column:"api_url"`
	Enabled bool   `column:"enabled"`
}

// :one
const countResources = `
select count(*) from resources where deleted_at is null
`

// CountResources runs an SQL query.
//
// select count(*) from resources where deleted_at is null
func CountResources() int64 {
	row := QueryRow(countResources)
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

// :exec
const disableResource = `
update resources set enabled = false where id = ?
`

// DisableResource runs an SQL query.
//
// update resources set enabled = false where id = ?
func DisableResource(id int64) {
	_, err := Exec(disableResource, id)
	if err != nil {
		handleError(err)
	}
}

// :one
const findResourceSummary = `
select id, api_url from resources where id = ?
`

type FindResourceSummaryRow struct {
	ID     int64
	APIURL string
}

// FindResourceSummary runs an SQL query.
//
// select id, api_url from resources where id = ?
func FindResourceSummary(id int64) (*FindResourceSummaryRow, bool) {
	row := QueryRow(findResourceSummary, id)
	item := new(FindResourceSummaryRow)

	err := row.Scan(&item.ID, &item.APIURL)
	if err == sql.ErrNoRows {
		return item, false
	}
	if err != nil {
		handleError(err)
		return item, false
	}
	return item, true
}

// :many
const findResourcesByIDs = `
select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources
where id in (/*SLICE:ids*/?)
order by id
`

// FindResourcesByIDs runs an SQL query.
//
// select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources where id in (/*SLICE:ids*/?) order by id
func FindResourcesByIDs(ids []int64) []*Resource {
	q := findResourcesByIDs
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
	var items []*Resource
	for rows.Next() {
		item := new(Resource)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// :execlastid
const insertAuditEntry = `
insert into audit_entries (occurred_on, duration, year_value, description)
values (?, ?, ?, ?)
`

type InsertAuditEntryParams struct {
	OccurredOn  time.Time
	Duration    sql.Null[time.Time]
	YearValue   int64
	Description string
}

// InsertAuditEntry runs an SQL query.
//
// insert into audit_entries (occurred_on, duration, year_value, description) values (?, ?, ?, ?)
func InsertAuditEntry(params InsertAuditEntryParams) int64 {
	result, err := Exec(insertAuditEntry,
		params.OccurredOn,
		params.Duration,
		params.YearValue,
		params.Description,
	)
	if err != nil {
		handleError(err)
		return 0
	}
	if lastInsertID, err := result.LastInsertId(); err != nil {
		handleError(err)
		return 0
	} else {
		return lastInsertID
	}
}

// :many
const listResourceURLs = `
select api_url from resources where deleted_at is null order by id
`

// ListResourceURLs runs an SQL query.
//
// select api_url from resources where deleted_at is null order by id
func ListResourceURLs() []string {
	rows, err := Query(listResourceURLs)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []string
	for rows.Next() {
		var api_url string
		if err := rows.Scan(&api_url); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, api_url)
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

// :many
const listResources = `
select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources where deleted_at is null order by id
`

// ListResources runs an SQL query.
//
// select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources where deleted_at is null order by id
func ListResources() []*Resource {
	rows, err := Query(listResources)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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

// :many
const searchResources = `
select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources
where enabled = ? and category = ? and api_url like ?
order by id
`

type SearchResourcesParams struct {
	Enabled  bool
	Category any
	APIURL   string
}

// SearchResources runs an SQL query.
//
// select id, created_at, deleted_at, api_url, payload, optional_payload, ratio, enabled, category from resources where enabled = ? and category = ? and api_url like ? order by id
func SearchResources(params SearchResourcesParams) []*Resource {
	rows, err := Query(searchResources, params.Enabled, params.Category, params.APIURL)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Resource
	for rows.Next() {
		item := new(Resource)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.APIURL,
			&item.Payload,
			&item.OptionalPayload,
			&item.Ratio,
			&item.Enabled,
			&item.Category,
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
const touchAuditEntry = `
update audit_entries set description = ? where id = ?
`

type TouchAuditEntryParams struct {
	Description string
	ID          int64
}

// TouchAuditEntry runs an SQL query.
//
// update audit_entries set description = ? where id = ?
func TouchAuditEntry(params TouchAuditEntryParams) sql.Result {
	result, err := Exec(touchAuditEntry, params.Description, params.ID)
	if err != nil {
		handleError(err)
	}
	return result
}

// :execrows
const updateResource = `
update resources
set api_url = ?, enabled = ?
where id = ?
`

type UpdateResourceParams struct {
	APIURL  string
	Enabled bool
	ID      int64
}

// UpdateResource runs an SQL query.
//
// update resources set api_url = ?, enabled = ? where id = ?
func UpdateResource(params UpdateResourceParams) int64 {
	result, err := Exec(updateResource, params.APIURL, params.Enabled, params.ID)
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
