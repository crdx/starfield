package generate

import "testing"

func TestGeneratedDSNInterpolatesParams(t *testing.T) {
	runGeneratedTest(t, "dsn_test.go", dsnTest)
}

const dsnTest = `package db

import "testing"

func TestDSNInterpolateParams(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*DSN)
		want  string
	}{
		{"default", func(*DSN) {}, "/app?interpolateParams=true&parseTime=true"},
		{"disabled", func(dsn *DSN) { dsn.InterpolateParams = false }, "/app?parseTime=true"},
		{"safe charset", func(dsn *DSN) { dsn.Charset = "utf8mb4" }, "/app?charset=utf8mb4&interpolateParams=true&parseTime=true"},
		{"unsafe charset", func(dsn *DSN) { dsn.Charset = "GBK" }, "/app?charset=GBK&parseTime=true"},
	}
	for _, test := range tests {
		dsn := NewDSN()
		dsn.DBName = "app"
		test.apply(dsn)
		if got := dsn.Format(); got != test.want {
			t.Errorf("%s: Format() = %q, want %q", test.name, got, test.want)
		}
	}
}
`
