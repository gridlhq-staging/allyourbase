package jobs

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestMoviesReembedUpdateSQLDoesNotHardcodeVectorDimension(t *testing.T) {
	t.Parallel()

	query := moviesReembedUpdateSQL()
	testutil.True(t, strings.Contains(query, "::vector"))
	testutil.False(t, strings.Contains(query, "::vector("))
}
