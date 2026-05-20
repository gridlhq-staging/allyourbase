// Package replica contains helper functions for matching replica lag rows.
package replica

import (
	"fmt"
	"net/url"
	"strings"
)

var sensitiveReplicaURLQueryKeys = map[string]struct{}{
	"password":    {},
	"passfile":    {},
	"sslpassword": {},
	"user":        {},
}

type replicaHints struct {
	host            string
	applicationName string
}

func parseReplicaHints(rawURL string) (replicaHints, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return replicaHints{}, fmt.Errorf("parse replica URL: %w", err)
	}

	return replicaHints{
		host:            strings.ToLower(parsed.Hostname()),
		applicationName: parsed.Query().Get("application_name"),
	}, nil
}

// selectReplicationLagRow finds a replication lag row matching the given hints using a scoring system that prioritizes rows matching both host and application name. Returns false if no row matches or if multiple rows tie with the same score.
func selectReplicationLagRow(rows []replicationLagRow, hints replicaHints) (replicationLagRow, bool) {
	var (
		selected  replicationLagRow
		found     bool
		ambiguous bool
		bestScore int
	)

	for _, row := range rows {
		score := rowMatchScore(row, hints)
		if score == 0 {
			continue
		}

		if !found || score > bestScore {
			selected = row
			bestScore = score
			found = true
			ambiguous = false
			continue
		}

		if score == bestScore {
			ambiguous = true
		}
	}

	if !found || ambiguous {
		return replicationLagRow{}, false
	}

	return selected, true
}

func rowMatchScore(row replicationLagRow, hints replicaHints) int {
	score := 0
	if hints.applicationName != "" && row.ApplicationName == hints.applicationName {
		score++
	}
	if hints.host != "" && strings.EqualFold(row.ClientAddr, hints.host) {
		score++
	}
	return score
}

// SanitizeReplicaURL removes credentials and sensitive query parameters before
// a replica connection string crosses a log, metric, or admin-response boundary.
func SanitizeReplicaURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-replica-url>"
	}

	parsed.User = nil
	values := parsed.Query()
	for key := range values {
		if _, sensitive := sensitiveReplicaURLQueryKeys[strings.ToLower(strings.TrimSpace(key))]; sensitive {
			values.Del(key)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}
