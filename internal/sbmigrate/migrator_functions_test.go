package sbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFunctionIdentityDistinguishesOverloads(t *testing.T) {
	t.Parallel()

	integerIdentity := FunctionIdentity{
		SchemaName:        "public",
		Name:              "normalize",
		IdentityArguments: "integer",
	}
	textIdentity := FunctionIdentity{
		SchemaName:        "public",
		Name:              "normalize",
		IdentityArguments: "text",
	}

	testutil.Equal(t, "public.normalize(integer)", integerIdentity.QualifiedName())
	testutil.Equal(t, "public.normalize(text)", textIdentity.QualifiedName())
	testutil.False(t, integerIdentity.Key() == textIdentity.Key(), "overloaded functions must have distinct keys")
}

func TestClassifyFunctionUsesAdmittedSchemaPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema string
		action functionCatalogAction
		reason string
	}{
		{name: "public", schema: "public", action: functionCatalogMigrate},
		{name: "admitted non-public", schema: "billing", action: functionCatalogMigrate},
		{
			name:   "excluded user schema",
			schema: "auth",
			action: functionCatalogSkip,
			reason: "function belongs to excluded schema auth",
		},
		{name: "postgres catalog", schema: "pg_catalog", action: functionCatalogIgnore},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry := supportedFunctionCatalogEntry()
			entry.Identity.SchemaName = tt.schema

			classification := classifyFunctionCatalogEntry(entry)

			testutil.Equal(t, tt.action, classification.Action)
			testutil.Equal(t, tt.reason, classification.Reason)
			testutil.Equal(t, tt.action == functionCatalogMigrate, isAdmittedUserSchema(tt.schema))
		})
	}
}

func TestExcludedSchemaReferenceDetectionUsesIdentifierTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		definition string
		schema     string
		found      bool
	}{
		{
			name:       "unquoted excluded schema",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS 'SELECT auth.uid()' LANGUAGE sql`,
			schema:     "auth",
			found:      true,
		},
		{
			name:       "quoted excluded schema",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS 'SELECT "storage".owner()' LANGUAGE sql`,
			schema:     "storage",
			found:      true,
		},
		{
			name:       "net token",
			definition: `CREATE FUNCTION public.f() RETURNS text AS 'SELECT net.http_get()' LANGUAGE sql`,
			schema:     "net",
			found:      true,
		},
		{
			name:       "net is not a suffix match",
			definition: `CREATE FUNCTION planet.f() RETURNS text AS 'SELECT planet.name()' LANGUAGE sql`,
			found:      false,
		},
		{
			name: "line comment mention is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				-- auth.uid() is documentation only.
				SELECT gen_random_uuid();
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "block comment mention is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				/* auth.uid() is documentation only. */
				SELECT gen_random_uuid();
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name:       "literal mention is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS text AS $$ SELECT 'auth.uid()'::text; $$ LANGUAGE sql`,
			found:      false,
		},
		{
			name: "execute single quoted SQL is executable",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				BEGIN
					EXECUTE 'SELECT auth.uid()';
					RETURN gen_random_uuid();
				END;
			$$ LANGUAGE plpgsql`,
			schema: "auth",
			found:  true,
		},
		{
			name: "execute dollar quoted SQL is executable",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				BEGIN
					EXECUTE $sql$SELECT storage.owner()$sql$;
					RETURN gen_random_uuid();
				END;
			$$ LANGUAGE plpgsql`,
			schema: "storage",
			found:  true,
		},
		{
			name: "nested literal in execute SQL is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS text AS $$
				BEGIN
					EXECUTE 'SELECT ''auth.uid()''::text';
					RETURN 'ok';
				END;
			$$ LANGUAGE plpgsql`,
			found: false,
		},
		{
			name: "nested comment in execute SQL is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS text AS $$
				BEGIN
					EXECUTE 'SELECT 1 /* storage.owner() is documentation only. */';
					RETURN 'ok';
				END;
			$$ LANGUAGE plpgsql`,
			found: false,
		},
		{
			name: "nested line comment in execute SQL is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS text AS $$
				BEGIN
					EXECUTE 'SELECT 1 -- auth.uid() is documentation only.
					';
					RETURN 'ok';
				END;
			$$ LANGUAGE plpgsql`,
			found: false,
		},
		{
			name: "nested dollar quoted literal in execute SQL is not executable",
			definition: `CREATE FUNCTION public.f() RETURNS text AS $function$
				BEGIN
					EXECUTE $sql$SELECT $value$auth.uid()$value$::text$sql$;
					RETURN 'ok';
				END;
			$function$ LANGUAGE plpgsql`,
			found: false,
		},
		{
			name: "excluded schema name used as table alias",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				SELECT auth.id FROM public.users AS auth LIMIT 1;
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "excluded schema name used as implicit table alias",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				SELECT auth.id FROM public.users auth LIMIT 1;
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "alias does not hide function call",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				SELECT auth.uid() FROM public.users auth LIMIT 1;
			$$ LANGUAGE sql`,
			schema: "auth",
			found:  true,
		},
		{
			name: "alias is scoped to its statement",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				SELECT auth.id FROM public.users auth LIMIT 1;
				SELECT auth.uid();
			$$ LANGUAGE sql`,
			schema: "auth",
			found:  true,
		},
		{
			name: "CTE name used as qualifier",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				WITH auth AS (SELECT gen_random_uuid() AS id)
				SELECT auth.id FROM auth;
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "recursive CTE name used as qualifier",
			definition: `CREATE FUNCTION public.f() RETURNS integer AS $$
				WITH RECURSIVE auth AS (
					SELECT 1 AS id
					UNION ALL
					SELECT auth.id + 1 FROM auth WHERE auth.id < 3
				)
				SELECT max(auth.id) FROM auth;
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "CTE name with column list used as qualifier",
			definition: `CREATE FUNCTION public.f() RETURNS uuid AS $$
				WITH auth(id) AS (VALUES (gen_random_uuid()))
				SELECT auth.id FROM auth;
			$$ LANGUAGE sql`,
			found: false,
		},
		{
			name: "CTE name does not hide excluded schema relation",
			definition: `CREATE FUNCTION public.f() RETURNS bigint AS $$
				WITH auth AS (SELECT 1)
				SELECT count(*) FROM auth.users;
			$$ LANGUAGE sql`,
			schema: "auth",
			found:  true,
		},
		{
			name: "quoted CTE name does not hide quoted excluded schema relation",
			definition: `CREATE FUNCTION public.f() RETURNS bigint AS $$
				WITH "auth" AS (SELECT 1)
				SELECT count(*) FROM "auth"."users";
			$$ LANGUAGE sql`,
			schema: "auth",
			found:  true,
		},
		{
			name: "CTE name does not hide comma separated excluded schema relation",
			definition: `CREATE FUNCTION public.f() RETURNS bigint AS $$
				WITH auth AS (SELECT 1)
				SELECT count(*) FROM public.accounts a, auth.users u;
			$$ LANGUAGE sql`,
			schema: "auth",
			found:  true,
		},
		{
			name:       "excluded schema in parameter default",
			definition: `CREATE FUNCTION public.f(subject uuid DEFAULT auth.uid()) RETURNS uuid AS 'SELECT subject' LANGUAGE sql`,
			schema:     "auth",
			found:      true,
		},
		{
			name:       "excluded schema text in parameter default literal",
			definition: `CREATE FUNCTION public.f(label text DEFAULT 'auth.uid()') RETURNS text AS 'SELECT label' LANGUAGE sql`,
			found:      false,
		},
		{
			name:       "excluded schema text in quoted parameter name",
			definition: `CREATE FUNCTION public.f("auth.uid()" text) RETURNS text AS 'SELECT "auth.uid()"' LANGUAGE sql`,
			found:      false,
		},
		{
			name:       "excluded schema text in quoted function name",
			definition: `CREATE FUNCTION public."auth.uid()"() RETURNS text AS 'SELECT ''ok''' LANGUAGE sql`,
			found:      false,
		},
		{
			name: "body delimiter text in parameter default literal",
			definition: `CREATE OR REPLACE FUNCTION public.f(label text DEFAULT 'AS $$'::text)
			 RETURNS uuid
			 LANGUAGE plpgsql
			AS $function$
			BEGIN
				RETURN auth.uid();
			END;
			$function$`,
			schema: "auth",
			found:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema, found := excludedSchemaReference(tt.definition)
			testutil.Equal(t, tt.found, found)
			testutil.Equal(t, tt.schema, schema)
		})
	}
}

func TestClassifyFunctionReportsUnsupportedCatalogRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*functionCatalogEntry)
		reason string
	}{
		{
			name: "malformed catalog row",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = ""
			},
			reason: "function catalog row is malformed",
		},
		{
			name: "extension owned",
			mutate: func(entry *functionCatalogEntry) {
				entry.ExtensionOwned = true
			},
			reason: "function is owned by an extension",
		},
		{
			name: "aggregate",
			mutate: func(entry *functionCatalogEntry) {
				entry.Kind = "a"
			},
			reason: "aggregate functions are not supported",
		},
		{
			name: "window",
			mutate: func(entry *functionCatalogEntry) {
				entry.Kind = "w"
			},
			reason: "window functions are not supported",
		},
		{
			name: "operator implementation",
			mutate: func(entry *functionCatalogEntry) {
				entry.OperatorImplementation = true
			},
			reason: "operator implementation functions are not supported",
		},
		{
			name: "procedure",
			mutate: func(entry *functionCatalogEntry) {
				entry.Kind = "p"
			},
			reason: "procedures are not supported",
		},
		{
			name: "C language",
			mutate: func(entry *functionCatalogEntry) {
				entry.Language = "c"
			},
			reason: "function language c is not supported",
		},
		{
			name: "internal language",
			mutate: func(entry *functionCatalogEntry) {
				entry.Language = "internal"
			},
			reason: "function language internal is not supported",
		},
		{
			name: "unknown language",
			mutate: func(entry *functionCatalogEntry) {
				entry.Language = "plpython3u"
			},
			reason: "function language plpython3u is not supported",
		},
		{
			name: "excluded schema body reference",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = `CREATE FUNCTION public.f() RETURNS uuid AS 'SELECT auth.uid()' LANGUAGE sql`
			},
			reason: "function definition references excluded schema auth",
		},
		{
			name: "comment-only excluded schema mention",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = `CREATE FUNCTION public.f() RETURNS uuid AS $$
					-- auth.uid() is documentation only.
					SELECT gen_random_uuid();
				$$ LANGUAGE sql`
			},
		},
		{
			name: "literal-only excluded schema mention",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = `CREATE FUNCTION public.f() RETURNS text AS $$ SELECT 'auth.uid()'::text; $$ LANGUAGE sql`
			},
		},
		{
			name: "dynamic SQL excluded schema reference",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = `CREATE FUNCTION public.f() RETURNS uuid AS $$
					BEGIN
						EXECUTE 'SELECT auth.uid()';
						RETURN gen_random_uuid();
					END;
				$$ LANGUAGE plpgsql`
			},
			reason: "function definition references excluded schema auth",
		},
		{
			name: "header default excluded schema reference",
			mutate: func(entry *functionCatalogEntry) {
				entry.Definition = `CREATE FUNCTION public.f(subject uuid DEFAULT auth.uid()) RETURNS uuid AS 'SELECT subject' LANGUAGE sql`
			},
			reason: "function definition references excluded schema auth",
		},
		{
			name: "table composite signature dependency",
			mutate: func(entry *functionCatalogEntry) {
				entry.CompositeTypeDependency = "billing.invoice"
				entry.CompositeTypeRelationKind = "r"
			},
			reason: "function signature references table-defined composite type billing.invoice",
		},
		{
			name: "view composite signature dependency",
			mutate: func(entry *functionCatalogEntry) {
				entry.CompositeTypeDependency = "billing.invoice_view"
				entry.CompositeTypeRelationKind = "v"
			},
			reason: "function signature references view-defined composite type billing.invoice_view",
		},
		{
			name: "materialized view composite signature dependency",
			mutate: func(entry *functionCatalogEntry) {
				entry.CompositeTypeDependency = "billing.invoice_summary"
				entry.CompositeTypeRelationKind = "m"
			},
			reason: "function signature references view-defined composite type billing.invoice_summary",
		},
		{
			name: "standalone composite signature dependency",
			mutate: func(entry *functionCatalogEntry) {
				entry.CompositeTypeDependency = "billing.invoice_payload"
				entry.CompositeTypeRelationKind = "c"
			},
			reason: "function signature references pre-schema composite type billing.invoice_payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry := supportedFunctionCatalogEntry()
			tt.mutate(&entry)

			classification := classifyFunctionCatalogEntry(entry)

			if tt.reason == "" {
				testutil.Equal(t, functionCatalogMigrate, classification.Action)
				testutil.Equal(t, "", classification.Reason)
				return
			}
			testutil.Equal(t, functionCatalogSkip, classification.Action)
			testutil.Equal(t, tt.reason, classification.Reason)
			testutil.Equal(t, 1, functionCatalogDenominator([]functionCatalogEntry{entry}))
		})
	}
}

func supportedFunctionCatalogEntry() functionCatalogEntry {
	return functionCatalogEntry{
		Identity: FunctionIdentity{
			SchemaName: "public",
			Name:       "answer",
		},
		Definition: `CREATE FUNCTION public.answer() RETURNS integer AS 'SELECT 42' LANGUAGE sql`,
		Kind:       "f",
		Language:   "sql",
	}
}
