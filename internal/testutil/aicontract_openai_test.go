//go:build aicontract

package testutil

import "testing"

func TestPickOpenAIContractModel(t *testing.T) {
	t.Run("prefers gpt 4o mini family when present", func(t *testing.T) {
		got, ok := pickOpenAIContractModel([]string{
			"gpt-4o",
			"gpt-4o-mini-2024-07-18",
			"gpt-3.5-turbo",
		})
		if !ok {
			t.Fatal("expected model selection to succeed")
		}
		if got != "gpt-4o-mini-2024-07-18" {
			t.Fatalf("model = %q; want gpt-4o-mini-2024-07-18", got)
		}
	})

	t.Run("filters non chat variants before fallback", func(t *testing.T) {
		got, ok := pickOpenAIContractModel([]string{
			"gpt-realtime-2.1-mini",
			"gpt-4-turbo",
			"gpt-3.5-turbo-instruct",
		})
		if !ok {
			t.Fatal("expected model selection to succeed")
		}
		if got != "gpt-4-turbo" {
			t.Fatalf("model = %q; want gpt-4-turbo", got)
		}
	})

	t.Run("accepts chatgpt fallback models", func(t *testing.T) {
		got, ok := pickOpenAIContractModel([]string{
			"chatgpt-image-latest",
			"chatgpt-4o-latest",
		})
		if !ok {
			t.Fatal("expected model selection to succeed")
		}
		if got != "chatgpt-4o-latest" {
			t.Fatalf("model = %q; want chatgpt-4o-latest", got)
		}
	})

	t.Run("accepts o series fallback models", func(t *testing.T) {
		got, ok := pickOpenAIContractModel([]string{
			"o1-mini",
			"gpt-image-1",
		})
		if !ok {
			t.Fatal("expected model selection to succeed")
		}
		if got != "o1-mini" {
			t.Fatalf("model = %q; want o1-mini", got)
		}
	})

	t.Run("fails when no chat candidate remains", func(t *testing.T) {
		if _, ok := pickOpenAIContractModel([]string{
			"text-embedding-3-small",
			"gpt-realtime-2",
			"chatgpt-image-latest",
			"whisper-1",
		}); ok {
			t.Fatal("expected model selection to fail")
		}
	})
}
