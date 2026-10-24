package z2m_test

import (
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/z2m"
)

// TestNewPropFilterReportsEveryUnsupportedPattern covers NewPropFilter's
// error: it names each pattern Go's regexp can't compile, not only the first,
// and the filter keeps the patterns that compiled.
func TestNewPropFilterReportsEveryUnsupportedPattern(t *testing.T) {
	filter, err := z2m.NewPropFilter([]string{"(?!a)", "battery", "(?<=b)"})

	require.Error(t, err)
	var syntaxErr *syntax.Error
	require.ErrorAs(t, err, &syntaxErr, "the regexp errors stay reachable")
	assert.Contains(t, err.Error(), `"(?!a)"`)
	assert.Contains(t, err.Error(), `"(?<=b)"`)
	assert.NotContains(t, err.Error(), `"battery"`)
	assert.True(t, filter.Match("battery"))
}
