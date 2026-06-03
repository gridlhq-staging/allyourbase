package pgmanager

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const contractProbePrefix = "CONTRACT_DOWNLOAD_URL="

// TestContractProbeDownloadURL is a machine-readable contract seam used by
// tests/contract/pg_arm64_asset_arch.sh to derive the release asset URL from
// pgmanager behavior instead of parsing source text formatting.
func TestContractProbeDownloadURL(t *testing.T) {
	version := strings.TrimSpace(os.Getenv("AYB_CONTRACT_PG_VERSION"))
	platform := strings.TrimSpace(os.Getenv("AYB_CONTRACT_PG_PLATFORM"))
	if version == "" || platform == "" {
		t.Skip("contract probe env not set")
	}

	url := downloadURL("", version, platform)
	expectedSuffix := fmt.Sprintf("/ayb-postgres-%s-%s.tar.xz", version, platform)
	if !strings.HasPrefix(url, "https://github.com/") {
		t.Fatalf("contract probe URL must use GitHub release host: %s", url)
	}
	if !strings.Contains(url, "/releases/download/pg-"+version+"/") {
		t.Fatalf("contract probe URL missing versioned release path: %s", url)
	}
	if !strings.HasSuffix(url, expectedSuffix) {
		t.Fatalf("contract probe URL missing expected archive suffix %q: %s", expectedSuffix, url)
	}

	fmt.Printf("%s%s\n", contractProbePrefix, url)
}
