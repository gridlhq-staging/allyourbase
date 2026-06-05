package api

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildSearchIndexSpecLongUnicodeNameIsValidUTF8(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema: strings.Repeat("a", 36),
		Name:   "測" + strings.Repeat("b", 32),
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "body", Position: 1, TypeName: "text"},
		},
	}

	spec, ok, err := buildSearchIndexSpec(tbl, "english")
	testutil.NoError(t, err)
	testutil.Equal(t, true, ok)
	if !utf8.ValidString(spec.name) {
		t.Fatalf("search index name must be valid UTF-8, got bytes %x", []byte(spec.name))
	}
	if len(spec.name) > 63 {
		t.Fatalf("search index name length = %d bytes, want <= 63", len(spec.name))
	}
}
