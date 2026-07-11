//go:build aicontract

package ai

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

func resolveContractProvider(t *testing.T, providerName string, cfg config.AIConfig) (Provider, string) {
	t.Helper()

	reg, err := BuildRegistry(cfg, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}

	provider, model, err := ResolveProvider(reg, providerName, "", "", cfg)
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	return provider, model
}

func requireContractEnv(t *testing.T, name string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s must be set for aicontract tests", name)
	}
	return value
}

func contractContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func assertExactContractText(t *testing.T, got, want string) {
	t.Helper()

	if strings.TrimSpace(got) != want {
		t.Fatalf("response text = %q; want exactly %q", got, want)
	}
}

type contractStreamResult struct {
	Text          string
	NonEmptyReads int
}

func readContractStream(t *testing.T, stream io.Reader) contractStreamResult {
	t.Helper()

	buf := make([]byte, 4096)
	var out []byte
	nonEmptyReads := 0
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			nonEmptyReads++
			out = append(out, buf[:n]...)
		}
		if errors.Is(err, io.EOF) {
			return contractStreamResult{
				Text:          string(out),
				NonEmptyReads: nonEmptyReads,
			}
		}
		if err != nil {
			t.Fatalf("Read stream: %v", err)
		}
	}
}

func assertTextUsage(t *testing.T, usage Usage) {
	t.Helper()

	if usage.InputTokens == 0 {
		t.Fatal("Usage.InputTokens = 0; want non-zero")
	}
	if usage.OutputTokens == 0 {
		t.Fatal("Usage.OutputTokens = 0; want non-zero")
	}
}
