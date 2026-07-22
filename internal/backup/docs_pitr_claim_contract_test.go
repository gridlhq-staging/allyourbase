package backup

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/allyourbase/ayb/internal/pgmanager"
	_ "unsafe"
)

//go:linkname writeManagedPostgresConf github.com/allyourbase/ayb/internal/pgmanager.writePostgresConf
func writeManagedPostgresConf(dataDir string, port uint32, runtimeDir string, sharedPreloadLibraries []string, archiveCommand string) error

// retiredWALArchivalClaim is the specific untrue promise this contract removed.
// It must never reappear, regardless of what the config generator emits.
const retiredWALArchivalClaim = "continuously archives PostgreSQL write-ahead log"

// managedPostgresAutomaticWALAnchor and externalPostgresManualWALAnchor keep
// ownership explicit for the two supported PostgreSQL deployment modes.
const managedPostgresAutomaticWALAnchor = "For AYB-managed PostgreSQL, `ayb start` automatically enables WAL archival"
const externalPostgresManualWALAnchor = "For externally managed PostgreSQL, you must install the archive settings yourself"

// retiredOperatorManagedWALAnchor is the pre-automation contract that must not
// reappear now that managed startup owns the archive command.
const retiredOperatorManagedWALAnchor = "does not configure Postgres to invoke `ayb wal-ship`"

// retiredShadowModeAutomaticWALClaim is the shadow-mode wording that implied
// AYB archives WAL automatically. Shadow mode only gates restore cutover.
const retiredShadowModeAutomaticWALClaim = "In shadow mode, AYB archives WAL segments and takes base backups normally"

// shadowModeArchiveContinuityAnchor states what shadow mode does not gate.
const shadowModeArchiveContinuityAnchor = "`shadow_mode` does not disable base backups or WAL shipping"

// retiredFireDrillClaims promise that a fire drill executes a restore.
// FireDrillRunner.Run only asks RestorePlanner.ValidateWindow for a plan.
var retiredFireDrillClaims = []string{
	"works end-to-end",
	"attempts a PITR restore",
}

// fireDrillPlanOnlyAnchor is the stable phrase bounding a fire drill to plan validation.
const fireDrillPlanOnlyAnchor = "validates only that a restore plan is viable"

func TestDocsPITRClaim(t *testing.T) {
	docs := readBackupsGuide(t)
	lowerDocs := strings.ToLower(docs)

	if !strings.Contains(docs, fireDrillPlanOnlyAnchor) {
		t.Errorf("backups guide is missing the plan-only fire-drill anchor %q", fireDrillPlanOnlyAnchor)
	}
	for _, claim := range retiredFireDrillClaims {
		if strings.Contains(lowerDocs, strings.ToLower(claim)) {
			t.Errorf("backups guide still claims a fire drill %q; FireDrillRunner.Run only validates a restore plan", claim)
		}
	}

	if !strings.Contains(docs, managedPostgresAutomaticWALAnchor) {
		t.Errorf("backups guide is missing the managed-Postgres automatic WAL anchor %q", managedPostgresAutomaticWALAnchor)
	}
	if !strings.Contains(docs, externalPostgresManualWALAnchor) {
		t.Errorf("backups guide is missing the external-Postgres manual WAL anchor %q", externalPostgresManualWALAnchor)
	}
	if strings.Contains(docs, retiredOperatorManagedWALAnchor) {
		t.Errorf("backups guide still contains the retired operator-managed WAL contract %q", retiredOperatorManagedWALAnchor)
	}
	if strings.Contains(docs, retiredWALArchivalClaim) {
		t.Errorf("backups guide still makes the retired automatic-archival promise %q", retiredWALArchivalClaim)
	}
	if !strings.Contains(docs, shadowModeArchiveContinuityAnchor) {
		t.Errorf("backups guide is missing the shadow-mode archive-continuity anchor %q", shadowModeArchiveContinuityAnchor)
	}
	if strings.Contains(lowerDocs, strings.ToLower(retiredShadowModeAutomaticWALClaim)) {
		t.Errorf("backups guide still claims shadow mode auto-archives WAL with %q", retiredShadowModeAutomaticWALClaim)
	}

	if !generatedPostgresConfHasArchiveCommand(t) {
		t.Error("managed-Postgres config generator has no active archive_command for a usable PITR configuration")
	}
}

// TestDocsSiteFireDrillClaims sweeps every published docs-site page, not just the
// backup guide, so the retired execution claims cannot reappear on another page.
// Untracked plans, archived roadmaps, and handoffs are deliberately out of scope:
// they are historical records that legitimately quote the old wording.
func TestDocsSiteFireDrillClaims(t *testing.T) {
	docsSite := filepath.Join(repoRootForDocsPITRClaim(t), "docs-site")

	err := filepath.WalkDir(docsSite, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		page, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(strings.ToLower(string(page)), "fire drill") {
			return nil
		}
		lowerPage := strings.ToLower(string(page))
		for _, claim := range retiredFireDrillClaims {
			if strings.Contains(lowerPage, strings.ToLower(claim)) {
				t.Errorf("%s discusses fire drills and claims %q; FireDrillRunner.Run only validates a restore plan", path, claim)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs-site: %v", err)
	}
}

// generatedPostgresConfHasArchiveCommand reports whether the managed-Postgres
// config generator emits an active archive_command. It is the falsifiable half
// of the docs contract: if automatic archival ever lands, this flips to true and
// the automatic-claim assertions above stop failing.
func generatedPostgresConfHasArchiveCommand(t *testing.T) bool {
	t.Helper()

	dataDir := t.TempDir()
	runtimeDir := t.TempDir()
	archiveCommand := "/opt/ayb/bin/ayb wal-ship --config /etc/ayb/ayb.toml %p %f"
	if err := writeManagedPostgresConf(dataDir, 25432, runtimeDir, []string{"pg_stat_statements"}, archiveCommand); err != nil {
		t.Fatalf("write managed postgres config: %v", err)
	}
	conf, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	if err != nil {
		t.Fatalf("read generated postgres config: %v", err)
	}
	return postgresConfHasActiveSetting(string(conf), "archive_command")
}

func readBackupsGuide(t *testing.T) string {
	t.Helper()

	docsPath := filepath.Join(repoRootForDocsPITRClaim(t), "docs-site", "guide", "backups.md")
	docs, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("read docs PITR guide: %v", err)
	}
	return string(docs)
}

func postgresConfHasActiveSetting(conf, setting string) bool {
	lastValue := ""
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		value = strings.TrimSpace(value)
		if ok && strings.TrimSpace(name) == setting {
			lastValue = value
		}
	}
	return lastValue != "" && lastValue != "''" && lastValue != `""`
}

func TestPostgresConfHasActiveSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		conf    string
		setting string
		want    bool
	}{
		{
			name: "active setting present",
			conf: `
# archive_command = 'disabled'
archive_command = '/bin/true'
`,
			setting: "archive_command",
			want:    true,
		},
		{
			name: "later empty override disables setting",
			conf: `
archive_command = '/bin/true'
archive_command = ''
`,
			setting: "archive_command",
			want:    false,
		},
		{
			name: "later commented line does not override active setting",
			conf: `
archive_command = '/bin/true'
# archive_command = ''
`,
			setting: "archive_command",
			want:    true,
		},
		{
			name: "missing setting",
			conf: `
shared_buffers = '128MB'
`,
			setting: "archive_command",
			want:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := postgresConfHasActiveSetting(tc.conf, tc.setting)
			if got != tc.want {
				t.Fatalf("postgresConfHasActiveSetting() = %t, want %t for %q", got, tc.want, tc.name)
			}
		})
	}
}

func repoRootForDocsPITRClaim(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root containing .git")
		}
		dir = parent
	}
}
