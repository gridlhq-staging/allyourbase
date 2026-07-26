package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type errorWithSuggestions struct {
	message     string
	suggestions []string
}

func (e *errorWithSuggestions) Error() string {
	return e.message
}

func (e *errorWithSuggestions) Suggestions() []string {
	return append([]string(nil), e.suggestions...)
}

func commandInputError(message, usage, example string) error {
	return &errorWithSuggestions{
		message: fmt.Sprintf("%s\nUsage: %s\nExample: %s", message, usage, example),
		suggestions: []string{
			example,
		},
	}
}

func exactArgsWithHelp(count int, example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == count {
			return nil
		}
		usage := cmd.CommandPath()
		if _, argumentUsage, found := strings.Cut(strings.TrimSpace(cmd.Use), " "); found {
			usage += " " + strings.TrimSpace(argumentUsage)
		}
		return commandInputError(
			fmt.Sprintf("expected %d argument(s), received %d", count, len(args)),
			usage,
			example,
		)
	}
}
