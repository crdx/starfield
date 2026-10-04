package generate

import (
	"context"
	"go/format"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateQueryWithLeadingWhitespaceIsGofmtClean(t *testing.T) {
	request := getRequest(t)
	query := request.Queries[len(request.Queries)-1]
	query.Comments = []string{" Deactivates a user."}
	query.Text = "\n  " + query.Text + "\n"

	response, err := Run(context.Background(), request)
	require.NoError(t, err)

	code := string(response.Files[0].Contents)
	assert.Contains(t, code, "//\n// update users set active = 0 where id = ?\nfunc DeactivateUser(")
	assertGofmtClean(t, response.Files[0].Contents)
}

func assertGofmtClean(t *testing.T, code []byte) {
	t.Helper()

	formatted, err := format.Source(code)
	require.NoError(t, err)
	assert.Equal(t, string(formatted), string(code), "generated code is not gofmt-clean")
}
