package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestMetricsWarningDoesNotOwnDocsBaseURL(t *testing.T) {
	t.Parallel()

	value := findConstOrVarInitializer(t, "helpers.go", "metricsAuthTokenWarningMessage")
	if expressionContainsString(value, "https://allyourbase.io") {
		t.Fatal("metricsAuthTokenWarningMessage must build docs URLs with httputil.DocURL")
	}
}

func findConstOrVarInitializer(t *testing.T, path, name string) ast.Expr {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, ident := range valueSpec.Names {
				if ident.Name == name && index < len(valueSpec.Values) {
					return valueSpec.Values[index]
				}
			}
		}
	}
	t.Fatalf("%s initializer not found in %s", name, path)
	return nil
}

func expressionContainsString(expr ast.Expr, needle string) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if found {
			return false
		}
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		found = strings.Contains(value, needle)
		return !found
	})
	return found
}
