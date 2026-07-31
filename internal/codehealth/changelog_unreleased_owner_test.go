package codehealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// changelogUnreleasedOwnerTestFile is the single test permitted to extract and
// assert on the CONTENTS of the CHANGELOG.md [Unreleased] section.
const changelogUnreleasedOwnerTestFile = "changelog_unreleased_inventory_test.go"

// assertsUnreleasedSectionContents reports whether a Go test source extracts the
// [Unreleased] section in order to assert on what is inside it.
//
// Why this rule exists: [Unreleased] is the one changelog section whose contents
// change at every release, because release-prep empties it. Any test that pins
// its prose is therefore guaranteed to go red on release day, and it will be
// found at the worst possible moment — mid-release.
//
// That has now happened twice. The first was a version-pinned v0.0.17-beta
// guard, retired 2026-07-15 (see internal/docs/stage2_beta_honesty_docs_test.go
// for that note). The second was TestStage2V0020ReleasePreparationTruthDocumented,
// which required five bullets and six raw commit SHAs to be present inside
// [Unreleased]; it was found only by simulating the release promotion on
// 2026-07-30, and it directly contradicted the standing ROADMAP.md decision to
// remove those SHAs from published prose. Twice is a pattern, so it gets a guard.
//
// Released version sections are immutable history and may be pinned freely.
// This rule covers [Unreleased] alone. Merely asserting that the "## [Unreleased]"
// heading EXISTS is also fine — that does not break at release; only extracting
// the section to assert on its body does.
//
// Detection is line-scoped: the extraction helper and the heading literal must
// appear on one line, which is how both real occurrences were written. A caller
// who split the call across lines would evade it. That is an accepted limit of a
// source scan, recorded here rather than hidden.
func assertsUnreleasedSectionContents(source string) bool {
	for _, line := range strings.Split(source, "\n") {
		if strings.Contains(line, "markdownSectionByHeading") && strings.Contains(line, "[Unreleased]") {
			return true
		}
	}
	return false
}

func TestAssertsUnreleasedSectionContentsDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   bool
	}{
		{
			// The exact shape both real offenders had.
			name:   "extracts the section for assertion",
			source: "func T(t *testing.T) {\n\tu := markdownSectionByHeading(t, changelog, \"## [Unreleased]\")\n}\n",
			want:   true,
		},
		{
			// Asserting the heading exists does not break at release.
			name:   "only asserts the heading exists",
			source: "requireContainsAll(t, \"CHANGELOG.md\",\n\t\"## [Unreleased]\",\n)\n",
			want:   false,
		},
		{
			// Extracting a released section is legitimate: that content is frozen.
			name:   "extracts a released section",
			source: "v := markdownSectionByHeading(t, changelog, \"## [0.0.20-beta] - 2026-07-23\")\n",
			want:   false,
		},
		{
			name:   "unrelated source",
			source: "package docs\n\nfunc TestSomething(t *testing.T) {}\n",
			want:   false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := assertsUnreleasedSectionContents(tc.source); got != tc.want {
				t.Fatalf("assertsUnreleasedSectionContents = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOnlyOneTestOwnsChangelogUnreleasedContents(t *testing.T) {
	t.Parallel()
	docsDir := filepath.Join(findRepoRoot(t), "internal", "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("read internal/docs: %v", err)
	}

	var offenders []string
	ownerSeen := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(docsDir, name))
		if err != nil {
			t.Fatalf("read internal/docs/%s: %v", name, err)
		}
		if !assertsUnreleasedSectionContents(string(source)) {
			continue
		}
		if name == changelogUnreleasedOwnerTestFile {
			ownerSeen = true
			continue
		}
		offenders = append(offenders, name)
	}

	// Without this the scan would pass vacuously if the owner were renamed or
	// deleted: zero files asserting on [Unreleased] would look identical to
	// "everything is fine", and the release-inventory reconciliation would be
	// silently gone.
	if !ownerSeen {
		t.Fatalf(
			"internal/docs/%s no longer extracts the [Unreleased] section; "+
				"the release-inventory reconciliation has no owner",
			changelogUnreleasedOwnerTestFile,
		)
	}
	if len(offenders) > 0 {
		t.Fatalf(
			"these internal/docs tests pin CHANGELOG.md [Unreleased] contents and will go red at release-prep: %s. "+
				"Only %s may assert on that section. Pin released version sections instead — they are immutable.",
			strings.Join(offenders, ", "), changelogUnreleasedOwnerTestFile,
		)
	}
}
