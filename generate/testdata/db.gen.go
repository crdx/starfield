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

type Post struct {
	ID        uint64    `column:"id"`
	CreatedAt time.Time `column:"created_at"`
	UserID    uint64    `column:"user_id"`
	Title     string    `column:"title"`
	Body      string    `column:"body"`
	Published bool      `column:"published"`
}

// GetPostTableStatus returns the TableStatus for the Posts table.
func GetPostTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "posts"`)

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

// CreatePost creates a new instance of Post.
func CreatePost(value *Post) *Post {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = Now()
	}
	result, err := Exec("insert into `posts` (`id`, `created_at`, `user_id`, `title`, `body`, `published`) values (?, ?, ?, ?, ?, ?)",
		value.ID,
		value.CreatedAt,
		value.UserID,
		value.Title,
		value.Body,
		value.Published,
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
	value.ID = uint64(lastInsertID)

	return value
}

// FindPost finds a non-deleted instance of Post by ID.
func FindPost[T Integer | ~string](id T) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where id = ?", id)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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

// FindPostUnscoped finds an instance (including deleted) of Post by ID.
func FindPostUnscoped[T Integer | ~string](id T) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where id = ?", id)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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

// FindPosts finds all non-deleted instances of Post.
func FindPosts() []*Post {
	rows, err := Query("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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

// Reload reloads this instance of Post.
func (self *Post) Reload() {
	if s, found := FindPost(self.ID); found {
		*self = *s
	}
}

// FindPostsByUserID finds all non-deleted instances of Post by UserID.
func FindPostsByUserID(value uint64) []*Post {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `user_id` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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

// FindPostByUserID finds an instance of Post by UserID.
func FindPostByUserID(value uint64) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `user_id` = ?", value)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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

// UpdateUserID updates the UserID field.
func (self *Post) UpdateUserID(value uint64) bool {
	result, err := Exec("update `posts` set `user_id` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.UserID = value
	return rowsAffected > 0
}

// FindPostsByTitle finds all non-deleted instances of Post by Title.
func FindPostsByTitle(value string) []*Post {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `title` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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

// FindPostByTitle finds an instance of Post by Title.
func FindPostByTitle(value string) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `title` = ?", value)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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

// UpdateTitle updates the Title field.
func (self *Post) UpdateTitle(value string) bool {
	result, err := Exec("update `posts` set `title` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Title = value
	return rowsAffected > 0
}

// FindPostsByBody finds all non-deleted instances of Post by Body.
func FindPostsByBody(value string) []*Post {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `body` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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

// FindPostByBody finds an instance of Post by Body.
func FindPostByBody(value string) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `body` = ?", value)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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
func (self *Post) UpdateBody(value string) bool {
	result, err := Exec("update `posts` set `body` = ? where id = ?", value, self.ID)
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

// FindPostsByPublished finds all non-deleted instances of Post by Published.
func FindPostsByPublished(value bool) []*Post {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `published` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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

// FindPostByPublished finds an instance of Post by Published.
func FindPostByPublished(value bool) (*Post, bool) {
	row := QueryRow("select `id`, `created_at`, `user_id`, `title`, `body`, `published` from `posts` where `published` = ?", value)
	item := new(Post)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.UserID,
		&item.Title,
		&item.Body,
		&item.Published,
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

// UpdatePublished updates the Published field.
func (self *Post) UpdatePublished(value bool) bool {
	result, err := Exec("update `posts` set `published` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Published = value
	return rowsAffected > 0
}

type User struct {
	ID        uint64              `column:"id"`
	CreatedAt time.Time           `column:"created_at"`
	DeletedAt sql.Null[time.Time] `column:"deleted_at"`
	Name      string              `column:"name"`
	Email     sql.Null[string]    `column:"email"`
	Age       sql.Null[int64]     `column:"age"`
	Active    bool                `column:"active"`
	Balance   string              `column:"balance"`
	Score     sql.Null[float64]   `column:"score"`
	Notes     sql.Null[string]    `column:"notes"`
	Avatar    sql.Null[string]    `column:"avatar"`
	LastLogin sql.Null[time.Time] `column:"last_login"`
}

// GetUserTableStatus returns the TableStatus for the Users table.
func GetUserTableStatus() (TableStatus, error) {
	row := QueryRow(`SHOW TABLE STATUS LIKE "users"`)

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

// CreateUser creates a new instance of User.
func CreateUser(value *User) *User {
	if value.CreatedAt.IsZero() {
		value.CreatedAt = Now()
	}
	result, err := Exec("insert into `users` (`id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login`) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		value.ID,
		value.CreatedAt,
		value.DeletedAt,
		value.Name,
		value.Email,
		value.Age,
		value.Active,
		value.Balance,
		value.Score,
		value.Notes,
		value.Avatar,
		value.LastLogin,
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
	value.ID = uint64(lastInsertID)

	return value
}

// FindUser finds a non-deleted instance of User by ID.
func FindUser[T Integer | ~string](id T) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where id = ? and deleted_at is null", id)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// FindUserUnscoped finds an instance (including deleted) of User by ID.
func FindUserUnscoped[T Integer | ~string](id T) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where id = ?", id)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// FindUsers finds all non-deleted instances of User.
func FindUsers() []*User {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where deleted_at is null order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// Reload reloads this instance of User.
func (self *User) Reload() {
	if s, found := FindUser(self.ID); found {
		*self = *s
	}
}

// FindUsersUnscoped finds all instances (including deleted) of User.
func FindUsersUnscoped() []*User {
	rows, err := Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` order by id asc")
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// Delete soft deletes this instance of User.
func (self *User) Delete() bool {
	result, err := Exec("update `users` set deleted_at = ? where id = ?", Now(), self.ID)
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

// Restore restores this instance of User.
func (self *User) Restore() bool {
	result, err := Exec("update `users` set deleted_at = null where id = ?", self.ID)
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

// HardDelete hard deletes (i.e. DELETE) this instance of User.
func (self *User) HardDelete() bool {
	result, err := Exec("delete from `users` where id = ?", self.ID)
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

// FindUsersByName finds all non-deleted instances of User by Name.
func FindUsersByName(value string) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `name` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByNameUnscoped finds all instances (including deleted) of User by Name.
func FindUsersByNameUnscoped(value string) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `name` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByName finds an instance of User by Name.
func FindUserByName(value string) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `name` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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
func (self *User) UpdateName(value string) bool {
	result, err := Exec("update `users` set `name` = ? where id = ?", value, self.ID)
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

// FindUsersByEmail finds all non-deleted instances of User by Email.
func FindUsersByEmail(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `email` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `email` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByEmailUnscoped finds all instances (including deleted) of User by Email.
func FindUsersByEmailUnscoped(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `email` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `email` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByEmail finds an instance of User by Email.
func FindUserByEmail(value string) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `email` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateEmail updates the Email field.
func (self *User) UpdateEmail(value string) bool {
	result, err := Exec("update `users` set `email` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Email = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearEmail sets the Email field to NULL.
func (self *User) ClearEmail() bool {
	result, err := Exec("update `users` set `email` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Email = sql.Null[string]{}
	return rowsAffected > 0
}

// FindUsersByAge finds all non-deleted instances of User by Age.
func FindUsersByAge(value sql.Null[int64]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `age` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `age` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByAgeUnscoped finds all instances (including deleted) of User by Age.
func FindUsersByAgeUnscoped(value sql.Null[int64]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `age` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `age` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByAge finds an instance of User by Age.
func FindUserByAge(value int64) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `age` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateAge updates the Age field.
func (self *User) UpdateAge(value int64) bool {
	result, err := Exec("update `users` set `age` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Age = sql.Null[int64]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearAge sets the Age field to NULL.
func (self *User) ClearAge() bool {
	result, err := Exec("update `users` set `age` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Age = sql.Null[int64]{}
	return rowsAffected > 0
}

// FindUsersByActive finds all non-deleted instances of User by Active.
func FindUsersByActive(value bool) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `active` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByActiveUnscoped finds all instances (including deleted) of User by Active.
func FindUsersByActiveUnscoped(value bool) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `active` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByActive finds an instance of User by Active.
func FindUserByActive(value bool) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `active` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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
func (self *User) UpdateActive(value bool) bool {
	result, err := Exec("update `users` set `active` = ? where id = ?", value, self.ID)
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

// FindUsersByBalance finds all non-deleted instances of User by Balance.
func FindUsersByBalance(value string) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `balance` = ? AND deleted_at IS NULL order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByBalanceUnscoped finds all instances (including deleted) of User by Balance.
func FindUsersByBalanceUnscoped(value string) []*User {
	var rows *sql.Rows
	var err error
	rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `balance` = ? order by id asc", value)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByBalance finds an instance of User by Balance.
func FindUserByBalance(value string) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `balance` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateBalance updates the Balance field.
func (self *User) UpdateBalance(value string) bool {
	result, err := Exec("update `users` set `balance` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Balance = value
	return rowsAffected > 0
}

// FindUsersByScore finds all non-deleted instances of User by Score.
func FindUsersByScore(value sql.Null[float64]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `score` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `score` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByScoreUnscoped finds all instances (including deleted) of User by Score.
func FindUsersByScoreUnscoped(value sql.Null[float64]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `score` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `score` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByScore finds an instance of User by Score.
func FindUserByScore(value float64) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `score` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateScore updates the Score field.
func (self *User) UpdateScore(value float64) bool {
	result, err := Exec("update `users` set `score` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Score = sql.Null[float64]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearScore sets the Score field to NULL.
func (self *User) ClearScore() bool {
	result, err := Exec("update `users` set `score` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Score = sql.Null[float64]{}
	return rowsAffected > 0
}

// FindUsersByNotes finds all non-deleted instances of User by Notes.
func FindUsersByNotes(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `notes` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `notes` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByNotesUnscoped finds all instances (including deleted) of User by Notes.
func FindUsersByNotesUnscoped(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `notes` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `notes` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByNotes finds an instance of User by Notes.
func FindUserByNotes(value string) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `notes` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateNotes updates the Notes field.
func (self *User) UpdateNotes(value string) bool {
	result, err := Exec("update `users` set `notes` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Notes = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearNotes sets the Notes field to NULL.
func (self *User) ClearNotes() bool {
	result, err := Exec("update `users` set `notes` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Notes = sql.Null[string]{}
	return rowsAffected > 0
}

// FindUsersByAvatar finds all non-deleted instances of User by Avatar.
func FindUsersByAvatar(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `avatar` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `avatar` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByAvatarUnscoped finds all instances (including deleted) of User by Avatar.
func FindUsersByAvatarUnscoped(value sql.Null[string]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `avatar` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `avatar` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByAvatar finds an instance of User by Avatar.
func FindUserByAvatar(value string) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `avatar` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateAvatar updates the Avatar field.
func (self *User) UpdateAvatar(value string) bool {
	result, err := Exec("update `users` set `avatar` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Avatar = sql.Null[string]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearAvatar sets the Avatar field to NULL.
func (self *User) ClearAvatar() bool {
	result, err := Exec("update `users` set `avatar` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.Avatar = sql.Null[string]{}
	return rowsAffected > 0
}

// FindUsersByLastLogin finds all non-deleted instances of User by LastLogin.
func FindUsersByLastLogin(value sql.Null[time.Time]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `last_login` = ? and deleted_at is null order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `last_login` is null and deleted_at is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUsersByLastLoginUnscoped finds all instances (including deleted) of User by LastLogin.
func FindUsersByLastLoginUnscoped(value sql.Null[time.Time]) []*User {
	var rows *sql.Rows
	var err error
	if value.Valid {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `last_login` = ? order by id asc", value)
	} else {
		rows, err = Query("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `last_login` is null order by id asc")
	}
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// FindUserByLastLogin finds an instance of User by LastLogin.
func FindUserByLastLogin(value time.Time) (*User, bool) {
	row := QueryRow("select `id`, `created_at`, `deleted_at`, `name`, `email`, `age`, `active`, `balance`, `score`, `notes`, `avatar`, `last_login` from `users` where `last_login` = ? AND deleted_at IS NULL", value)
	item := new(User)
	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
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

// UpdateLastLogin updates the LastLogin field.
func (self *User) UpdateLastLogin(value time.Time) bool {
	result, err := Exec("update `users` set `last_login` = ? where id = ?", value, self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.LastLogin = sql.Null[time.Time]{V: value, Valid: true}
	return rowsAffected > 0
}

// ClearLastLogin sets the LastLogin field to NULL.
func (self *User) ClearLastLogin() bool {
	result, err := Exec("update `users` set `last_login` = null where id = ?", self.ID)
	if err != nil {
		handleError(err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		handleError(err)
		return false
	}
	self.LastLogin = sql.Null[time.Time]{}
	return rowsAffected > 0
}

// :one
const countUsers = `
select count(*) from users
`

// CountUsers runs an SQL query.
//
// select count(*) from users
func CountUsers() int64 {
	row := QueryRow(countUsers)
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
const deactivateUser = `
update users set active = 0 where id = ?
`

// DeactivateUser runs an SQL query.
//
// update users set active = 0 where id = ?
func DeactivateUser(id uint64) {
	_, err := Exec(deactivateUser, id)
	if err != nil {
		handleError(err)
	}
}

// :many
const fetchActiveUsers = `
select id, created_at, deleted_at, name, email, age, active, balance, score, notes, avatar, last_login from users where active = 1 and deleted_at is null
`

// FetchActiveUsers runs an SQL query.
//
// select id, created_at, deleted_at, name, email, age, active, balance, score, notes, avatar, last_login from users where active = 1 and deleted_at is null
func FetchActiveUsers() []*User {
	rows, err := Query(fetchActiveUsers)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*User
	for rows.Next() {
		item := new(User)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DeletedAt,
			&item.Name,
			&item.Email,
			&item.Age,
			&item.Active,
			&item.Balance,
			&item.Score,
			&item.Notes,
			&item.Avatar,
			&item.LastLogin,
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

// :one
const fetchUserByEmail = `
select id, created_at, deleted_at, name, email, age, active, balance, score, notes, avatar, last_login from users where email = ?
`

// FetchUserByEmail runs an SQL query.
//
// select id, created_at, deleted_at, name, email, age, active, balance, score, notes, avatar, last_login from users where email = ?
func FetchUserByEmail(email sql.Null[string]) (*User, bool) {
	row := QueryRow(fetchUserByEmail, email)
	item := new(User)

	err := row.Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DeletedAt,
		&item.Name,
		&item.Email,
		&item.Age,
		&item.Active,
		&item.Balance,
		&item.Score,
		&item.Notes,
		&item.Avatar,
		&item.LastLogin,
	)
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
const fetchPostsByUser = `
select id, created_at, user_id, title, body, published from posts where user_id = ? order by created_at desc
`

// FetchPostsByUser runs an SQL query.
//
// select id, created_at, user_id, title, body, published from posts where user_id = ? order by created_at desc
func FetchPostsByUser(userID uint64) []*Post {
	rows, err := Query(fetchPostsByUser, userID)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []*Post
	for rows.Next() {
		item := new(Post)

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.UserID,
			&item.Title,
			&item.Body,
			&item.Published,
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
const fetchPublishedPostTitles = `
select title from posts where published = 1
`

// FetchPublishedPostTitles runs an SQL query.
//
// select title from posts where published = 1
func FetchPublishedPostTitles() []string {
	rows, err := Query(fetchPublishedPostTitles)
	if err != nil {
		handleError(err)
		return nil
	}
	defer rows.Close() //nolint:errcheck
	var items []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			handleError(err)
			return nil
		}
		items = append(items, title)
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
