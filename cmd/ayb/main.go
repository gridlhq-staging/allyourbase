package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/cli"
	"github.com/allyourbase/ayb/internal/cli/ui"
	buildversion "github.com/allyourbase/ayb/internal/version"
)

type errorWithSuggestions interface {
	error
	Suggestions() []string
}

// Set by goreleaser at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.SetVersion(version, commit, date)
	buildversion.Set(version)
	if err := cli.Execute(); err != nil {
		fmt.Fprint(os.Stderr, renderTopLevelError(err))
		os.Exit(1)
	}
}

func renderTopLevelError(err error) string {
	var suggested errorWithSuggestions
	if errors.As(err, &suggested) {
		return ui.FormatError(suggested.Error(), suggested.Suggestions()...)
	}
	return ui.FormatError(err.Error())
}
