package codehealth

import (
	"os"
	"path/filepath"
	"testing"
)

// The hosted live-polls demo is a static Cloudflare Pages site with no API
// proxy, so it can only reach the backend when the production API base URL is
// baked into the bundle at build time via VITE_AYB_URL. Without it the client
// falls back to a value that cannot serve the AYB API from the static origin
// (POST /api/auth/register returns 405), and authenticated poll creation fails
// on the hosted page. The sibling instantsearch demo already bakes this value;
// this contract keeps the live-polls deploy lane at parity so the hosted demo
// talks to https://api.allyourbase.io rather than its own static origin.
func TestLivePollsDeployBakesProductionApiBaseURL(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "deploy_live_polls.yml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", workflowPath, err)
	}
	workflowContent := string(workflowData)

	requireContainsAll(t, workflowContent, []string{
		"VITE_AYB_URL=https://api.allyourbase.io npm run build",
		"pages deploy dist --project-name=ayb-demo-polls",
	})
}
