package version

import (
	"strings"
	"sync/atomic"
)

const defaultVersion = "dev"

var current atomic.Value

func init() {
	current.Store(defaultVersion)
}

// Get returns the current build version.
func Get() string {
	value, ok := current.Load().(string)
	if !ok || value == "" {
		return defaultVersion
	}
	return value
}

// Set updates the current build version. Empty values are ignored.
func Set(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	current.Store(trimmed)
}
