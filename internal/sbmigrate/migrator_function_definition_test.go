package sbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestExcludedSchemaReferenceIgnoresFalseBodyDelimitersInHeaderText(t *testing.T) {
	t.Parallel()

	definitions := map[string]string{
		"dollar quoted default literal": `CREATE FUNCTION public.f(label text DEFAULT $value$AS $$$value$)
			RETURNS uuid LANGUAGE plpgsql
			AS $function$ BEGIN RETURN auth.uid(); END; $function$`,
		"line comment": `CREATE FUNCTION public.f(label text DEFAULT 'safe' -- AS $$
			) RETURNS uuid LANGUAGE plpgsql
			AS $function$ BEGIN RETURN auth.uid(); END; $function$`,
		"nested block comment": `CREATE FUNCTION public.f(label text DEFAULT 'safe'
			/* outer /* nested */ AS $$ still commented */)
			RETURNS uuid LANGUAGE plpgsql
			AS $function$ BEGIN RETURN auth.uid(); END; $function$`,
		"quoted identifier": `CREATE FUNCTION public.f("AS $$" text DEFAULT 'safe')
			RETURNS uuid LANGUAGE plpgsql
			AS $function$ BEGIN RETURN auth.uid(); END; $function$`,
	}

	for name, definition := range definitions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			schema, found := excludedSchemaReference(definition)
			testutil.True(t, found, "real executable body must be classified")
			testutil.Equal(t, "auth", schema)
		})
	}
}
