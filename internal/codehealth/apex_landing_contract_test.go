package codehealth

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const apexLandingCanonicalRepo = "AllyourbaseHQ/allyourbase"

const (
	apexCheckoutAction = "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd"
	apexSetupGoAction  = "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16"
	apexWranglerAction = "cloudflare/wrangler-action@9acf94ace14e7dc412b076f2c5c20b8ce93c79cd"
)

var (
	apexGitHubHrefPattern = regexp.MustCompile(`href=["'](https://github\.com/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)?)["']`)
	apexGitHubTextPattern = regexp.MustCompile(`>\s*(github\.com/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)?)\s*</a>`)
	apexDemoURLPattern    = regexp.MustCompile(`https://([a-z0-9-]+)\.demo\.allyourbase\.io`)
	apexOwnerLabelPattern = regexp.MustCompile("`([a-z0-9-]+)\\.`")
)

func TestApexLandingGitHubLinkIsCanonicalPublicRepo(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	indexHTML := readApexLandingFile(t, repoRoot, "index.html")
	identities := extractApexGitHubIdentities(indexHTML)
	// This explicit floor prevents an empty fixture from making the regex contract pass vacuously.
	if len(identities) == 0 {
		t.Fatal("expected at least one GitHub href or visible anchor identity in apex landing page")
	}

	wantURL := "https://github.com/" + apexLandingCanonicalRepo
	wantText := "github.com/" + apexLandingCanonicalRepo
	assertApexStringSetEqual(t, "GitHub identities", identities, apexStringSet(wantURL, wantText))
}

func TestApexLandingCanonicalRepoMatchesDebbieProdIdentity(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	debbiePath := filepath.Join(repoRoot, ".debbie.toml")
	content, err := os.ReadFile(debbiePath)
	if errors.Is(err, fs.ErrNotExist) {
		// Public mirrors may omit Debbie metadata safely because the GitHub-link test remains unconditional.
		t.Skip(".debbie.toml absent in public mirror")
	}
	if err != nil {
		t.Fatalf("read %s: %v", debbiePath, err)
	}

	if got := parseDebbieProdGitHub(string(content)); got != apexLandingCanonicalRepo {
		t.Fatalf("[identity.prod] github = %q, want canonical repo %q", got, apexLandingCanonicalRepo)
	}
}

func TestApexLandingDirmapMatchesIndexHTML(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	indexDemos := extractApexDemoNames(readApexLandingFile(t, repoRoot, "index.html"))
	dirmapContent := readApexLandingFile(t, repoRoot, "DIRMAP.md")
	dirmapDemos := extractApexDirmapOwnerDemos(dirmapContent)

	if len(indexDemos) == 0 {
		t.Fatal("expected at least one *.demo.allyourbase.io link in apex landing page")
	}
	if len(dirmapDemos) == 0 {
		t.Fatal("expected at least one live subdomain owner in apex landing DIRMAP owner sentence")
	}

	if missing := apexSetDifference(indexDemos, dirmapDemos); len(missing) > 0 {
		t.Errorf("DIRMAP live-owner sentence is missing demos linked by index.html: %v", missing)
	}
	if extra := apexSetDifference(dirmapDemos, indexDemos); len(extra) > 0 {
		t.Errorf("DIRMAP live-owner sentence has extra demos not linked by index.html: %v", extra)
	}
	normalizedDirmap := strings.Join(strings.Fields(dirmapContent), " ")
	if strings.Contains(normalizedDirmap, "until a live deploy owner has landed") {
		t.Error("DIRMAP still claims a demo is waiting until a live deploy owner has landed")
	}
}

func TestApexLandingDemoLinksHaveDeployOwners(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	demos := extractApexDemoNames(readApexLandingFile(t, repoRoot, "index.html"))
	if len(demos) == 0 {
		t.Fatal("expected at least one *.demo.allyourbase.io link in apex landing page")
	}

	for _, demo := range sortedApexSet(demos) {
		demo := demo
		t.Run(demo, func(t *testing.T) {
			workflowPath := filepath.Join(repoRoot, ".github", "workflows", "deploy_live_"+demo+".yml")
			content, err := os.ReadFile(workflowPath)
			if err != nil {
				t.Fatalf("read deploy owner workflow %s: %v", workflowPath, err)
			}
			requireContainsAll(t, string(content), []string{"pages deploy"})
		})
	}
}

func TestApexLandingDeployWorkflow(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "deploy_apex_landing.yml")
	workflowText := readRepoText(t, workflowPath)
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(workflowText), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	requireApexWorkflowTriggers(t, workflow)
	if len(workflow.Jobs) != 2 {
		t.Fatalf("apex workflow jobs = %d, want exactly test-apex and deploy-apex", len(workflow.Jobs))
	}
	testJob, ok := workflow.Jobs["test-apex"]
	if !ok {
		t.Fatal("workflow is missing jobs.test-apex")
	}
	deployJob, ok := workflow.Jobs["deploy-apex"]
	if !ok {
		t.Fatal("workflow is missing jobs.deploy-apex")
	}
	if !jobNeeds(deployJob, "test-apex") {
		t.Fatal("deploy-apex must depend on test-apex")
	}
	if !workflowJobHasRunStep(workflow, "test-apex", "go test ./internal/codehealth -run ^TestApexLanding -count=1") {
		t.Fatal("test-apex must run the focused apex contract from the repository root without being skippable")
	}
	if githubActionsPermissionGrantsWrite(workflow.Permissions, "deployments") {
		t.Fatal("deploy_apex_landing.yml must not grant deployments: write at workflow scope")
	}
	if githubActionsPermissionGrantsWrite(testJob.Permissions, "deployments") {
		t.Fatal("test-apex must not have deployments: write")
	}
	if !githubActionsPermissionGrantsWrite(deployJob.Permissions, "deployments") {
		t.Fatal("deploy-apex must declare deployments: write at job scope")
	}
	requireApexWorkflowActionPins(t, testJob, deployJob)
	requireApexArtifactStep(t, deployJob)
	requireApexDeployStep(t, deployJob, workflowText)
}

func requireApexWorkflowTriggers(t *testing.T, workflow githubActionsWorkflow) {
	t.Helper()

	if len(workflow.On) != 2 {
		t.Fatalf("apex workflow trigger count = %d, want exactly push and workflow_dispatch", len(workflow.On))
	}
	dispatch, ok := workflow.On["workflow_dispatch"]
	if !ok || dispatch.Kind != yaml.MappingNode || len(dispatch.Content) != 0 {
		t.Fatal("apex workflow must declare workflow_dispatch: {}")
	}
	pushNode, ok := workflow.On["push"]
	if !ok {
		t.Fatal("apex workflow must declare a push trigger")
	}
	var push githubActionsPushTrigger
	if err := pushNode.Decode(&push); err != nil {
		t.Fatalf("decode apex push trigger: %v", err)
	}
	requireApexExactStrings(t, "push branches", push.Branches, []string{"main"})
	requireApexExactStrings(t, "push paths", push.Paths, []string{
		"examples/apex_landing/**",
		".github/workflows/deploy_apex_landing.yml",
	})
}

func requireApexWorkflowActionPins(t *testing.T, testJob, deployJob githubActionsJob) {
	t.Helper()

	requireJobUsesAction(t, testJob, apexCheckoutAction)
	requireJobUsesAction(t, testJob, apexSetupGoAction)
	requireJobUsesAction(t, deployJob, apexCheckoutAction)
	requireJobUsesAction(t, deployJob, apexWranglerAction)
	requireApexExactStrings(t, "test-apex action refs", githubActionsJobActionRefs(testJob), []string{
		apexCheckoutAction,
		apexSetupGoAction,
	})
	requireApexExactStrings(t, "deploy-apex action refs", githubActionsJobActionRefs(deployJob), []string{
		apexCheckoutAction,
		apexWranglerAction,
	})
}

func requireApexArtifactStep(t *testing.T, deployJob githubActionsJob) {
	t.Helper()

	for _, step := range deployJob.Steps {
		if step.Name != "Prepare apex artifact" {
			continue
		}
		if step.WorkingDirectory != "examples/apex_landing" {
			t.Fatalf("Prepare apex artifact working-directory = %q, want examples/apex_landing", step.WorkingDirectory)
		}
		if got, want := strings.TrimSpace(step.Run), "mkdir -p dist\ncp index.html dist/index.html"; got != want {
			t.Fatalf("Prepare apex artifact run = %q, want %q", got, want)
		}
		if githubActionsContinueOnErrorEnabled(step.ContinueOnError) || strings.TrimSpace(step.If) != "" {
			t.Fatal("Prepare apex artifact must not be skippable or ignore failures")
		}
		return
	}
	t.Fatal("deploy-apex is missing Prepare apex artifact")
}

func requireApexDeployStep(t *testing.T, deployJob githubActionsJob, workflowText string) {
	t.Helper()

	step, ok := findJobActionStep(deployJob, apexWranglerAction)
	if !ok {
		t.Fatalf("deploy-apex is missing %s", apexWranglerAction)
	}
	want := map[string]string{
		"apiToken":         "${{ secrets.CLOUDFLARE_API_TOKEN }}",
		"accountId":        "${{ secrets.CLOUDFLARE_ACCOUNT_ID }}",
		"command":          "pages deploy dist --project-name=ayb-demo-apex",
		"workingDirectory": "examples/apex_landing",
	}
	if len(step.With) != len(want) {
		t.Fatalf("wrangler inputs = %v, want exactly %v", step.With, want)
	}
	for name, value := range want {
		if step.With[name] != value {
			t.Fatalf("wrangler input %s = %q, want %q", name, step.With[name], value)
		}
	}
	if got := strings.Count(workflowText, "${{ secrets."); got != 2 {
		t.Fatalf("apex workflow secret references = %d, want only the two established Cloudflare secrets", got)
	}
}

func requireApexExactStrings(t *testing.T, description string, got, want []string) {
	t.Helper()

	if !equalApexStrings(got, want) {
		t.Fatalf("%s = %v, want %v", description, got, want)
	}
}

func TestApexLandingContractParsersKnownAnswer(t *testing.T) {
	t.Parallel()

	t.Run("well formed page and DIRMAP", func(t *testing.T) {
		page := `<a href="https://kanban.demo.allyourbase.io">Kanban</a>
<a href="https://movies.demo.allyourbase.io">Movies</a>
<a href="https://github.com/AllyourbaseHQ/allyourbase">github.com/AllyourbaseHQ/allyourbase</a>`
		dirmap := "Owner of the apex URL; the two demo subdomains (`kanban.`, `movies.`) remain owned by their demo apps."

		assertApexStringSetEqual(t, "GitHub identities", extractApexGitHubIdentities(page), apexStringSet(
			"https://github.com/AllyourbaseHQ/allyourbase",
			"github.com/AllyourbaseHQ/allyourbase",
		))
		assertApexStringSetEqual(t, "index demos", extractApexDemoNames(page), apexStringSet("kanban", "movies"))
		assertApexStringSetEqual(t, "DIRMAP owners", extractApexDirmapOwnerDemos(dirmap), apexStringSet("kanban", "movies"))
	})

	t.Run("wrong GitHub org is preserved for comparison", func(t *testing.T) {
		page := `<a href="https://github.com/WrongOrg/allyourbase">github.com/WrongOrg/allyourbase</a>`
		assertApexStringSetEqual(t, "GitHub identities", extractApexGitHubIdentities(page), apexStringSet(
			"https://github.com/WrongOrg/allyourbase",
			"github.com/WrongOrg/allyourbase",
		))
	})

	t.Run("Debbie prod identity uses exact github key", func(t *testing.T) {
		debbie := `[identity.dev]
github = "private/dev"

[identity.prod]
github_legacy = "FormerOrg/allyourbase"
github = "AllyourbaseHQ/allyourbase"

[identity.other]
github = "wrong/section"`
		if got := parseDebbieProdGitHub(debbie); got != "AllyourbaseHQ/allyourbase" {
			t.Fatalf("prod github identity = %q, want %q", got, "AllyourbaseHQ/allyourbase")
		}
	})

	t.Run("DIRMAP ignores demos outside owner sentence labels", func(t *testing.T) {
		page := `<a href="https://kanban.demo.allyourbase.io">Kanban</a>
<a href="https://movies.demo.allyourbase.io">Movies</a>`
		dirmap := "Owner of the apex URL; the demo subdomain (`kanban.`) remains owned by its demo app, while Movies is described elsewhere.\n\n| index.html | Links movies.demo.allyourbase.io. |"
		indexDemos := extractApexDemoNames(page)
		dirmapDemos := extractApexDirmapOwnerDemos(dirmap)

		assertApexStringSetEqual(t, "DIRMAP owners", dirmapDemos, apexStringSet("kanban"))
		if got := apexSetDifference(indexDemos, dirmapDemos); !equalApexStrings(got, []string{"movies"}) {
			t.Fatalf("missing DIRMAP owners = %v, want [movies]", got)
		}
	})

	t.Run("empty fixtures return empty sets", func(t *testing.T) {
		if got := extractApexGitHubIdentities(""); len(got) != 0 {
			t.Fatalf("GitHub identities = %v, want empty", sortedApexSet(got))
		}
		if got := extractApexDemoNames(""); len(got) != 0 {
			t.Fatalf("index demos = %v, want empty", sortedApexSet(got))
		}
		if got := extractApexDirmapOwnerDemos(""); len(got) != 0 {
			t.Fatalf("DIRMAP owners = %v, want empty", sortedApexSet(got))
		}
	})
}

func readApexLandingFile(t *testing.T, repoRoot, name string) string {
	t.Helper()

	path := filepath.Join(repoRoot, "examples", "apex_landing", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func extractApexGitHubIdentities(page string) map[string]struct{} {
	identities := make(map[string]struct{})
	for _, match := range apexGitHubHrefPattern.FindAllStringSubmatch(page, -1) {
		identities[match[1]] = struct{}{}
	}
	for _, match := range apexGitHubTextPattern.FindAllStringSubmatch(page, -1) {
		identities[match[1]] = struct{}{}
	}
	return identities
}

func extractApexDemoNames(page string) map[string]struct{} {
	demos := make(map[string]struct{})
	for _, match := range apexDemoURLPattern.FindAllStringSubmatch(page, -1) {
		demos[match[1]] = struct{}{}
	}
	return demos
}

func extractApexDirmapOwnerDemos(dirmap string) map[string]struct{} {
	demos := make(map[string]struct{})
	for _, paragraph := range strings.Split(dirmap, "\n\n") {
		if !strings.Contains(paragraph, "Owner of the apex URL;") {
			continue
		}
		for _, match := range apexOwnerLabelPattern.FindAllStringSubmatch(paragraph, -1) {
			demos[match[1]] = struct{}{}
		}
		break
	}
	return demos
}

func parseDebbieProdGitHub(content string) string {
	inProdIdentity := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inProdIdentity = line == "[identity.prod]"
			continue
		}
		key, _, hasValue := strings.Cut(line, "=")
		if inProdIdentity && hasValue && strings.TrimSpace(key) == "github" {
			return firstQuotedValue(line)
		}
	}
	return ""
}

func apexStringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func assertApexStringSetEqual(t *testing.T, description string, got, want map[string]struct{}) {
	t.Helper()

	missing := apexSetDifference(want, got)
	extra := apexSetDifference(got, want)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("%s mismatch\nmissing: %v\nextra: %v", description, missing, extra)
	}
}

func apexSetDifference(source, target map[string]struct{}) []string {
	var difference []string
	for value := range source {
		if _, ok := target[value]; !ok {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}

func sortedApexSet(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func equalApexStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
