package algoliamigrate

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/searchsynonyms"
)

func TestMapAlgoliaSynonymsAcceptsOnlyRegularSynonymFixture(t *testing.T) {
	t.Parallel()

	input := loadSynonymFixtureInput(t)
	plan := MapAlgoliaSynonyms(input)

	wantGroups := searchsynonyms.Groups{{
		Terms: []string{"desk supplies", "office supplies", "stationery"},
	}}
	if !reflect.DeepEqual(plan.Groups, wantGroups) {
		t.Fatalf("groups mismatch:\nwant: %#v\n got: %#v", wantGroups, plan.Groups)
	}
	if plan.Stats.SupportedGroups != 1 {
		t.Fatalf("supported groups = %d, want 1", plan.Stats.SupportedGroups)
	}

	wantUnsupported := map[string]int{
		"oneWaySynonym":  1,
		"altCorrection1": 1,
		"altCorrection2": 1,
		"placeholder":    1,
	}
	if !reflect.DeepEqual(plan.Stats.SkippedUnsupportedTypes, wantUnsupported) {
		t.Fatalf("unsupported counts mismatch:\nwant: %#v\n got: %#v", wantUnsupported, plan.Stats.SkippedUnsupportedTypes)
	}
	wantReasons := map[string]string{
		"oneWaySynonym":  "unsupported directional synonym",
		"altCorrection1": "unsupported alternative correction",
		"altCorrection2": "unsupported alternative correction",
		"placeholder":    "unsupported placeholder synonym",
	}
	if !reflect.DeepEqual(plan.Stats.SkippedUnsupportedReasons, wantReasons) {
		t.Fatalf("unsupported reasons mismatch:\nwant: %#v\n got: %#v", wantReasons, plan.Stats.SkippedUnsupportedReasons)
	}
}

func TestMapAlgoliaSynonymsSkipsInvalidAndMalformedRegularGroups(t *testing.T) {
	t.Parallel()

	plan := MapAlgoliaSynonyms(SynonymInput{Hits: []AlgoliaSynonymHit{
		{Type: "synonym", Synonyms: []string{"only-one"}},
		{Type: "synonym"},
		{Type: "", Synonyms: []string{"valid", "terms"}},
		{Type: "synonym", Synonyms: []string{"Monitor", "Display"}},
	}})

	wantGroups := searchsynonyms.Groups{{Terms: []string{"display", "monitor"}}}
	if !reflect.DeepEqual(plan.Groups, wantGroups) {
		t.Fatalf("groups mismatch:\nwant: %#v\n got: %#v", wantGroups, plan.Groups)
	}
	if plan.Stats.SupportedGroups != 1 {
		t.Fatalf("supported groups = %d, want 1", plan.Stats.SupportedGroups)
	}
	if plan.Stats.SkippedInvalidGroups != 2 {
		t.Fatalf("invalid groups = %d, want 2", plan.Stats.SkippedInvalidGroups)
	}
	if plan.Stats.SkippedMalformedHits != 1 {
		t.Fatalf("malformed hits = %d, want 1", plan.Stats.SkippedMalformedHits)
	}
	if plan.Stats.SkippedInvalidReasons["each synonym group must include at least two terms"] != 2 {
		t.Fatalf("invalid reasons = %#v", plan.Stats.SkippedInvalidReasons)
	}
}

func TestMapAlgoliaSynonymsKeepsValidGroupsWhenLaterGroupDuplicatesTerm(t *testing.T) {
	t.Parallel()

	plan := MapAlgoliaSynonyms(SynonymInput{Hits: []AlgoliaSynonymHit{
		{Type: "synonym", Synonyms: []string{"Desk Lamp", "Task Light"}},
		{Type: "synonym", Synonyms: []string{"desk lamp", "Office Lamp"}},
		{Type: "synonym", Synonyms: []string{"Notebook", "Journal"}},
	}})

	wantGroups := searchsynonyms.Groups{
		{Terms: []string{"desk lamp", "task light"}},
		{Terms: []string{"journal", "notebook"}},
	}
	if !reflect.DeepEqual(plan.Groups, wantGroups) {
		t.Fatalf("groups mismatch:\nwant: %#v\n got: %#v", wantGroups, plan.Groups)
	}
	if plan.Stats.SupportedGroups != 2 {
		t.Fatalf("supported groups = %d, want 2", plan.Stats.SupportedGroups)
	}
	if plan.Stats.SkippedInvalidGroups != 1 {
		t.Fatalf("invalid groups = %d, want 1", plan.Stats.SkippedInvalidGroups)
	}
	if plan.Stats.SkippedInvalidReasons["duplicate synonym term: desk lamp"] != 1 {
		t.Fatalf("invalid reasons = %#v", plan.Stats.SkippedInvalidReasons)
	}
}

func loadSynonymFixtureInput(t *testing.T) SynonymInput {
	t.Helper()

	raw, err := os.ReadFile("testdata/algolia_synonyms_sample.json")
	if err != nil {
		t.Fatalf("read synonym fixture: %v", err)
	}
	var input SynonymInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatalf("decode synonym fixture: %v", err)
	}
	return input
}
