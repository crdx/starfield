package generate

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

var update = flag.Bool("update", false, "regenerate the golden test fixtures")

const (
	requestPath = "testdata/request.json"
	fixturePath = "testdata/db.gen.go"
	fixtureFile = "db.gen.go"
)

type goldenTest struct {
	Name        string
	RequestPath string
	FixturePath string
}

var goldenTests = []goldenTest{
	{
		Name:        "mysql-models",
		RequestPath: requestPath,
		FixturePath: fixturePath,
	},
	{
		Name:        "mysql-combinations",
		RequestPath: "testdata/goldens/mysql-combinations/request.json",
		FixturePath: "testdata/goldens/mysql-combinations/db/db.gen.go",
	},
}

func TestGenerate(t *testing.T) {
	for _, golden := range goldenTests {
		t.Run(golden.Name, func(t *testing.T) {
			testGenerateGolden(t, golden)
		})
	}
}

func testGenerateGolden(t *testing.T, golden goldenTest) {
	t.Helper()

	request := getRequestFromPath(t, golden.RequestPath)
	response, err := Run(context.Background(), request)
	require.NoError(t, err)

	require.Len(t, response.Files, 1)
	require.Equal(t, fixtureFile, response.Files[0].Name)

	code := response.Files[0].Contents
	if *update {
		require.NoError(t, os.WriteFile(golden.FixturePath, code, 0o600))
	}

	expected, err := os.ReadFile(golden.FixturePath)
	require.NoError(t, err)
	if !bytes.Equal(expected, code) {
		t.Errorf(
			"generated output does not match %s. ensure working tree is clean, then rerun with -update to see what changes",
			golden.FixturePath,
		)
	}

	assertCompiles(t, code)
}

func getRequest(t *testing.T) *plugin.GenerateRequest {
	t.Helper()
	return getRequestFromPath(t, requestPath)
}

func getRequestFromPath(t *testing.T, path string) *plugin.GenerateRequest {
	t.Helper()

	contents, err := os.ReadFile(path) //nolint:gosec // Golden paths are fixed by this package.
	require.NoError(t, err)

	var request plugin.GenerateRequest
	require.NoError(t, protojson.Unmarshal(contents, &request))

	return &request
}

func assertCompiles(t *testing.T, code []byte) {
	t.Helper()

	dir := t.TempDir()
	goMod := fmt.Sprintf("module fixture\n\ngo %s\n", goVersion(t))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, fixtureFile), code, 0o600))

	command := exec.Command("go", "build", "./...")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")

	output, err := command.CombinedOutput()
	assert.NoError(t, err, "generated code does not compile:\n%s", output)
}

func goVersion(t *testing.T) string {
	t.Helper()

	output, err := exec.Command("go", "list", "-m", "-f", "{{ .GoVersion }}").Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(output))
}
