//go:build integration

package api_test

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestVectorRankingSemanticQueryKnownAnswerIntegration(t *testing.T) {
	ctx := context.Background()
	h := setupVectorRankingHandler(t, ctx)

	cases := []struct {
		name          string
		distance      string
		wantIDs       []string
		wantDistances []float64
	}{
		{
			name:          "cosine",
			distance:      "cosine",
			wantIDs:       []string{"C", "D", "B", "A"},
			wantDistances: []float64{0, 0.01941932430908, 0.10557280900008414, 0.29289321881345254},
		},
		{
			name:          "l2",
			distance:      "l2",
			wantIDs:       []string{"A", "C", "B", "D"},
			wantDistances: []float64{1, math.Sqrt2, 2, math.Sqrt(5)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/collections/vector_rank_docs/?semantic_query=fruit+tech&distance=" + tc.distance
			w := doAPIHandlerRequest(t, h, http.MethodGet, path, nil)
			testutil.StatusCode(t, http.StatusOK, w.Code)

			body := parseJSON(t, w)
			items := jsonItems(t, body)
			testutil.Equal(t, 4, len(items))
			testutil.Equal(t, 4.0, jsonNum(t, body["totalItems"]))
			assertVectorRankingItems(t, items, tc.wantIDs, tc.wantDistances)
		})
	}
}

func setupVectorRankingHandler(t *testing.T, ctx context.Context) http.Handler {
	t.Helper()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS vector;
		CREATE TABLE vector_rank_docs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			embedding vector(3) NOT NULL
		);
		INSERT INTO vector_rank_docs (id, title, embedding) VALUES
			('A', 'fruit baseline', '[0,1,0]'),
			('B', 'fruit with tech tail', '[1,3,0]'),
			('C', 'balanced fruit tech', '[2,2,0]'),
			('D', 'tech-heavy fruit', '[2,3,0]');
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	h := api.NewHandler(sharedPG.Pool, ch, logger, nil, nil, nil)
	h.ApplyOptions(api.WithEmbedder(testVectorRankingEmbedder(t)))
	return h.Routes()
}

func testVectorRankingEmbedder(t *testing.T) api.EmbedFunc {
	t.Helper()
	return func(_ context.Context, texts []string) ([][]float64, error) {
		if len(texts) != 1 {
			t.Fatalf("embedding inputs = %v, want one text", texts)
		}
		// Test vocabulary dimensions are [fruit, tech, sport]. Token counts are
		// case-insensitive, so "fruit tech" embeds to [1,1,0] and other query
		// texts vary with their observed tokens instead of using a constant vector.
		vocab := map[string]int{"fruit": 0, "tech": 1, "sport": 2}
		out := make([]float64, 3)
		for _, token := range strings.Fields(strings.ToLower(texts[0])) {
			if idx, ok := vocab[token]; ok {
				out[idx]++
			}
		}
		return [][]float64{out}, nil
	}
}

func assertVectorRankingItems(t *testing.T, items []map[string]any, wantIDs []string, wantDistances []float64) {
	t.Helper()
	if len(items) != len(wantIDs) {
		t.Fatalf("item count = %d, want %d", len(items), len(wantIDs))
	}
	for i, item := range items {
		testutil.Equal(t, wantIDs[i], jsonStr(t, item["id"]))
		gotDistance, ok := item["_distance"].(float64)
		testutil.True(t, ok, "expected _distance to be numeric, got %T", item["_distance"])
		if math.Abs(gotDistance-wantDistances[i]) > 1e-6 {
			t.Fatalf("item %d (%s) _distance = %.12f, want %.12f", i, wantIDs[i], gotDistance, wantDistances[i])
		}
	}
}
