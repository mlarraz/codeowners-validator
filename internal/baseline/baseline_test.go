package baseline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mlarraz/codeowners-validator/internal/baseline"
)

func TestWriteAndLoad(t *testing.T) {
	// given: keys that YAML would misread unquoted
	path := filepath.Join(t.TempDir(), "baseline.yml")
	b := baseline.Baseline{
		"files":           {"/z", "*.js", "/a", "*.js", "[abc]/x", "#not-a-comment", "key: value", "yes", "~"},
		"avoid-shadowing": {"/docs/"},
		"duppatterns":     {},
	}

	// when
	require.NoError(t, baseline.Write(path, b))
	got, err := baseline.Load(path)

	// then
	require.NoError(t, err)
	assert.Equal(t, baseline.Baseline{
		"files":           {"#not-a-comment", "*.js", "/a", "/z", "[abc]/x", "key: value", "yes", "~"},
		"avoid-shadowing": {"/docs/"},
	}, got, "keys are sorted and unique, and checks without keys are left out")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(content)
	assert.True(t, strings.HasPrefix(text, "# Known CODEOWNERS issues"), "the file explains itself")
	assert.Less(t, strings.Index(text, "avoid-shadowing:"), strings.Index(text, "files:"), "checks are sorted")
}

func TestLoadEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yml")
	require.NoError(t, baseline.Write(path, baseline.Baseline{}))

	got, err := baseline.Load(path)

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := baseline.Load(filepath.Join(dir, "missing.yml"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	invalid := filepath.Join(dir, "invalid.yml")
	require.NoError(t, os.WriteFile(invalid, []byte("files: /a\n"), 0o600))
	_, err = baseline.Load(invalid)
	assert.ErrorContains(t, err, "while parsing the baseline")
}
