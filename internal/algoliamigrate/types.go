// Package algoliamigrate owns the non-CLI Algolia record import engine.
package algoliamigrate

import (
	"context"
	"net/http"

	"github.com/allyourbase/ayb/internal/searchsynonyms"
)

// Record is one Algolia hit decoded with json.Decoder.UseNumber.
type Record map[string]any

// BrowseConfig contains only the v1 browse settings the importer owns.
type BrowseConfig struct {
	Endpoint   string
	AppID      string
	APIKey     string
	IndexName  string
	HTTPClient *http.Client
}

// BrowseResult captures source facts from Algolia browse enumeration.
type BrowseResult struct {
	Records  []Record
	Requests int
}

// BrowseResponse is the documented Algolia browse envelope.
type BrowseResponse struct {
	Hits   []Record `json:"hits"`
	Cursor string   `json:"cursor,omitempty"`
}

// ColumnType is the PostgreSQL type family inferred for one target column.
type ColumnType string

const (
	ColumnTypeText    ColumnType = "text"
	ColumnTypeInteger ColumnType = "bigint"
	ColumnTypeDouble  ColumnType = "double precision"
	ColumnTypeBoolean ColumnType = "boolean"
	ColumnTypeJSONB   ColumnType = "jsonb"
)

// Column describes one inferred target column.
type Column struct {
	Name       string
	Type       ColumnType
	Nullable   bool
	PrimaryKey bool
}

// Schema is the pure analysis result for browsed Algolia records.
type Schema struct {
	Columns     []Column
	RecordCount int
}

// ImportOptions controls pure planning and transactional import behavior.
type ImportOptions struct {
	TargetSchema  string
	TargetTable   string
	DryRun        bool
	BatchSize     int
	Synonyms      *SynonymInput
	SynonymClient SynonymSearcher
}

// SourceFacts describes the Algolia source facts that later CLI code reports.
type SourceFacts struct {
	RecordCount int
}

// TargetPlan is the deterministic PostgreSQL plan derived from analysis.
type TargetPlan struct {
	SchemaName     string
	TableName      string
	Columns        []Column
	CreateTableSQL string
	PreflightSQL   string
	InsertSQL      string
}

// DryRunStats are the planned counts available without writing to the target.
type DryRunStats struct {
	TablesPlanned  int
	RecordsPlanned int
}

// ImportPlan keeps source facts, inferred schema, target SQL, and dry-run
// stats separate so CLI/reporting layers do not need to parse SQL internals.
type ImportPlan struct {
	Source   SourceFacts
	Schema   Schema
	Target   TargetPlan
	DryRun   DryRunStats
	Synonyms SynonymPlan
}

// ImportStats is the machine-readable result of a record import.
type ImportStats struct {
	Tables   int          `json:"tables"`
	Records  int          `json:"records"`
	Skipped  int          `json:"skipped,omitempty"`
	Errors   []string     `json:"errors,omitempty"`
	DryRun   bool         `json:"dryRun,omitempty"`
	Synonyms SynonymStats `json:"synonyms,omitempty,omitzero"`
}

// ParityResult compares source browse count and inserted target count.
type ParityResult struct {
	Source        string `json:"source"`
	SourceRecords int    `json:"sourceRecords"`
	TargetRecords int    `json:"targetRecords"`
	Match         bool   `json:"match"`
}

// SynonymSearcher is the optional Algolia settings-ACL client used by imports.
type SynonymSearcher interface {
	SearchSynonyms(ctx context.Context) (*SynonymInput, error)
}

// AlgoliaSynonymHit is one decoded Algolia synonym search hit.
type AlgoliaSynonymHit struct {
	ObjectID     string   `json:"objectID,omitempty"`
	Type         string   `json:"type,omitempty"`
	Synonyms     []string `json:"synonyms,omitempty"`
	Input        string   `json:"input,omitempty"`
	Word         string   `json:"word,omitempty"`
	Placeholder  string   `json:"placeholder,omitempty"`
	Corrections  []string `json:"corrections,omitempty"`
	Replacements []string `json:"replacements,omitempty"`
}

// SynonymInput captures enumerated Algolia synonym hits plus client-side facts.
type SynonymInput struct {
	Hits  []AlgoliaSynonymHit `json:"hits"`
	Stats SynonymStats        `json:"-"`
}

// SynonymPlan is the AYB-ready synonym output derived from Algolia hits.
type SynonymPlan struct {
	Groups searchsynonyms.Groups
	Stats  SynonymStats
}

// SynonymStats reports supported and skipped synonym carry-over paths.
type SynonymStats struct {
	SupportedGroups           int               `json:"supportedGroups,omitempty"`
	Requests                  int               `json:"requests,omitempty"`
	SkippedUnsupportedTypes   map[string]int    `json:"skippedUnsupportedTypes,omitempty"`
	SkippedUnsupportedReasons map[string]string `json:"skippedUnsupportedReasons,omitempty"`
	SkippedInvalidGroups      int               `json:"skippedInvalidGroups,omitempty"`
	SkippedInvalidReasons     map[string]int    `json:"skippedInvalidReasons,omitempty"`
	SkippedSettingsACL        int               `json:"skippedSettingsAcl,omitempty"`
	SkippedMalformedHits      int               `json:"skippedMalformedHits,omitempty"`
}

func (s SynonymStats) IsZero() bool {
	return s.SupportedGroups == 0 &&
		s.Requests == 0 &&
		len(s.SkippedUnsupportedTypes) == 0 &&
		len(s.SkippedUnsupportedReasons) == 0 &&
		s.SkippedInvalidGroups == 0 &&
		len(s.SkippedInvalidReasons) == 0 &&
		s.SkippedSettingsACL == 0 &&
		s.SkippedMalformedHits == 0
}
