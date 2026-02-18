package sbmigrate

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewMigratorMissingSourceURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		TargetURL: "postgres://localhost/ayb",
	})
	testutil.ErrorContains(t, err, "source database URL is required")
}

func TestNewMigratorMissingTargetURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourceURL: "postgres://localhost/supabase",
	})
	testutil.ErrorContains(t, err, "target database URL is required")
}

func TestNewMigratorBadSourceURL(t *testing.T) {
	t.Parallel()
	_, err := NewMigrator(MigrationOptions{
		SourceURL: "postgres://invalid:5432/nonexistent",
		TargetURL: "postgres://localhost/ayb",
	})
	testutil.ErrorContains(t, err, "source database")
}

func TestExtractString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data map[string]any
		keys []string
		want string
	}{
		{
			name: "first key match",
			data: map[string]any{"sub": "12345", "email": "a@b.com"},
			keys: []string{"sub"},
			want: "12345",
		},
		{
			name: "second key fallback",
			data: map[string]any{"full_name": "John Doe"},
			keys: []string{"name", "full_name"},
			want: "John Doe",
		},
		{
			name: "no match returns empty",
			data: map[string]any{"sub": "12345"},
			keys: []string{"email"},
			want: "",
		},
		{
			name: "empty string value skipped",
			data: map[string]any{"name": "", "full_name": "Jane"},
			keys: []string{"name", "full_name"},
			want: "Jane",
		},
		{
			name: "non-string value skipped",
			data: map[string]any{"sub": 12345},
			keys: []string{"sub"},
			want: "",
		},
		{
			name: "nil map returns empty",
			data: nil,
			keys: []string{"sub"},
			want: "",
		},
		{
			name: "no keys returns empty",
			data: map[string]any{"sub": "12345"},
			keys: []string{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractString(tt.data, tt.keys...)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestPrintStats(t *testing.T) {
	t.Parallel()
	t.Run("includes data fields when non-zero", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Tables:     5,
				Views:      2,
				Records:    1000,
				Sequences:  3,
				Users:      10,
				OAuthLinks: 5,
				Policies:   2,
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Tables:     5")
		testutil.Contains(t, out, "Views:      2")
		testutil.Contains(t, out, "Records:    1000")
		testutil.Contains(t, out, "Sequences:  3")
		testutil.Contains(t, out, "Users:      10")
		testutil.Contains(t, out, "OAuth:      5")
		testutil.Contains(t, out, "RLS:        2")
	})

	t.Run("omits data fields when zero", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Users: 10,
			},
		}
		m.printStats()
		out := buf.String()
		testutil.False(t, strings.Contains(out, "Tables:"), "should not show Tables when zero")
		testutil.False(t, strings.Contains(out, "Views:"), "should not show Views when zero")
		testutil.False(t, strings.Contains(out, "Records:"), "should not show Records when zero")
		testutil.False(t, strings.Contains(out, "Sequences:"), "should not show Sequences when zero")
		testutil.Contains(t, out, "Users:      10")
	})

	t.Run("shows errors", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		m := &Migrator{
			output: &buf,
			stats: MigrationStats{
				Errors: []string{"something went wrong"},
			},
		}
		m.printStats()
		out := buf.String()
		testutil.Contains(t, out, "Errors:     1")
		testutil.Contains(t, out, "something went wrong")
	})
}

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips user:pass", "postgres://user:secret@db.xxx.supabase.co:5432/postgres", "postgres://db.xxx.supabase.co:5432/postgres"},
		{"strips user only", "postgres://user@localhost:5432/db", "postgres://localhost:5432/db"},
		{"no credentials unchanged", "postgres://localhost:5432/db", "postgres://localhost:5432/db"},
		{"non-URL returns as-is", "not a url %%", "not a url %%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := redactURL(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}
