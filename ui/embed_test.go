package ui

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
)

func TestEmbeddedDistIncludesFunctionLogSelectors(t *testing.T) {
	jsBundle, err := readEmbeddedJS()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skip("embedded UI asset bundle not found; build ui/dist assets to validate selector markers")
		}
		t.Fatalf("read embedded UI bundle: %v", err)
	}

	for _, marker := range []string{
		"log-row-",
		"log-method-",
		"log-path-",
	} {
		if !strings.Contains(jsBundle, marker) {
			t.Fatalf("embedded UI bundle missing %q; rebuild ui/dist before go build", marker)
		}
	}
}

func TestEmbeddedDistIncludesOIDCProviderSelectors(t *testing.T) {
	jsBundle, err := readEmbeddedJS()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skip("embedded UI asset bundle not found; build ui/dist assets to validate selector markers")
		}
		t.Fatalf("read embedded UI bundle: %v", err)
	}

	for _, marker := range []string{
		"provider-delete-",
		"provider-form-issuer-url",
		"provider-form-display-name",
		"provider-form-scopes",
	} {
		if !strings.Contains(jsBundle, marker) {
			t.Fatalf("embedded UI bundle missing %q; rebuild ui/dist before go build", marker)
		}
	}
}

// readEmbeddedJS concatenates every embedded JS asset. The console is
// code-split, so screen selector markers live in lazy chunks rather than the
// index-* entry bundle.
func readEmbeddedJS() (string, error) {
	entries, err := fs.ReadDir(DistDirFS, "assets")
	if err != nil {
		return "", err
	}

	var all strings.Builder
	found := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".js") {
			continue
		}
		raw, readErr := fs.ReadFile(DistDirFS, path.Join("assets", name))
		if readErr != nil {
			return "", readErr
		}
		all.Write(raw)
		found = true
	}
	if !found {
		return "", fs.ErrNotExist
	}

	return all.String(), nil
}
