package pgconf

import "testing"

func TestEscapeStringLiteralDoublesSingleQuotes(t *testing.T) {
	t.Parallel()

	got := EscapeStringLiteral("cp /wal archive/config's/%f %p")
	want := "cp /wal archive/config''s/%f %p"
	if got != want {
		t.Fatalf("EscapeStringLiteral() = %q; want %q", got, want)
	}
}
