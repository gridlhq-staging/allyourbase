package codehealth

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const publishedDockerImageRepository = "ghcr.io/griddlehq/allyourbase"
const publishedDockerImageRef = publishedDockerImageRepository + ":latest"

var retryableDockerManifestInspectSignatures = []string{
	"could not resolve host",
	"temporary failure in name resolution",
	"no such host",
	"i/o timeout",
	"net/http: request canceled",
	"tls handshake timeout",
	"service unavailable",
	"too many requests",
	"429",
	"500 internal server error",
	"502 bad gateway",
	"503 service unavailable",
	"504 gateway timeout",
}

var dockerManifestDaemonUnavailableSignatures = []string{
	"cannot connect to the docker daemon",
	"is the docker daemon running",
	"error during connect",
	"permission denied while trying to connect to the docker daemon socket",
}

type dockerManifestInspectResponse struct {
	Manifests []struct {
		Platform dockerManifestPlatform `json:"platform"`
	} `json:"manifests"`
}

type dockerManifestPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

func TestPublishedDockerImageBindsToAllInterfaces(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, "Dockerfile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	content := string(data)
	requireContainsAll(t, content, []string{
		`ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]`,
		`ENV AYB_SERVER_HOST=0.0.0.0`,
		`CMD ["ayb", "start", "--foreground"]`,
	})
}

func TestReleaseEvidenceArtifactsStayIgnored(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	requireContainsAll(t, string(data), []string{
		"_dev/release/evidence/*",
	})
}

func TestPublishedDockerImageBuildsAllEmbeddedDemoDistAssets(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	embedPath := filepath.Join(repoRoot, "examples", "embed.go")
	embedData, err := os.ReadFile(embedPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", embedPath, err)
	}
	embedContent := string(embedData)
	requireContainsAll(t, embedContent, []string{
		"//go:embed kanban/dist live-polls/dist movies/dist",
	})

	dockerfilePath := filepath.Join(repoRoot, "Dockerfile")
	dockerfileData, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read %s failed: %v", dockerfilePath, err)
	}
	dockerfileContent := string(dockerfileData)

	requiredDemoDistDirs := []string{
		"kanban/dist",
		"live-polls/dist",
		"movies/dist",
	}
	for _, distDir := range requiredDemoDistDirs {
		demoName := strings.TrimSuffix(distDir, "/dist")
		requireContainsAll(t, dockerfileContent, []string{
			"WORKDIR /src/examples/" + demoName,
			"COPY examples/" + demoName + "/package*.json ./",
			"COPY examples/" + demoName + "/ .",
			"RUN VITE_AYB_URL=\"\" npx vite build",
			"COPY --from=demo-builder /src/examples/" + demoName + "/dist ./examples/" + demoName + "/dist",
		})
	}
}

func TestCIDockerSmokeWorkflowContract(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", workflowPath, err)
	}
	workflowContent := string(workflowData)

	requireContainsAll(t, workflowContent, []string{
		"docker-smoke:",
		"  docker-smoke:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v6",
		"      - name: Build Docker image (no-cache plain-progress evidence)\n        shell: bash\n        run: DOCKER_BUILDKIT=1 docker build --no-cache --progress=plain . 2>&1 | tee /tmp/docker-smoke-build.log",
		"DOCKER_BUILDKIT=1 docker build --no-cache --progress=plain . 2>&1 | tee /tmp/docker-smoke-build.log",
		"if: always()",
		"uses: actions/upload-artifact@v7",
		"name: docker-smoke-build-log",
		"path: /tmp/docker-smoke-build.log",
	})
}

func TestPublishedDockerfileCopiesFlyConfigFromBuildContext(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	dockerfilePath := filepath.Join(repoRoot, "Dockerfile")
	dockerfileData, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read %s failed: %v", dockerfilePath, err)
	}
	dockerfileContent := string(dockerfileData)

	requireContainsAll(t, dockerfileContent, []string{
		"deploy/fly/ayb.toml /home/ayb/ayb.toml",
	})
	requireDoesNotContainAny(t, dockerfileContent, []string{
		"/src/deploy/fly/ayb.toml",
		"COPY --from=builder --chown=ayb:ayb /src/deploy/fly/ayb.toml /home/ayb/ayb.toml",
	})
}

func TestDebbieHooksRehydrateFlyConfigForDockerBuildContext(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	requiredSnippets := []string{
		"$DEV_ROOT/deploy/fly/ayb.toml",
		"$TARGET_ROOT/deploy/fly/ayb.toml",
	}

	for _, relativePath := range []string{
		".debbie/post-sync-staging.sh",
		".debbie/post-sync-prod.sh",
	} {
		path := filepath.Join(repoRoot, relativePath)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// .debbie/ is dev-repo-only — debbie strips it from staging/prod mirrors,
				// so this PR-time grep-level contract cannot run there. The end-to-end
				// equivalent is the ci.yml `docker-smoke` job, which catches the same
				// regression by failing the `docker build .` step.
				t.Skipf("skipping on mirror without .debbie/: %s", path)
			}
			t.Fatalf("read %s failed: %v", path, err)
		}
		requireContainsAll(t, string(data), requiredSnippets)
	}
}

func TestPublishedDockerImageManifestIncludesArm64(t *testing.T) {
	t.Parallel()

	if os.Getenv("AYB_GHCR_MULTIARCH_GATE") != "1" {
		t.Skip("set AYB_GHCR_MULTIARCH_GATE=1 to run live GHCR multi-arch manifest contract")
	}

	manifestOutput, observedPlatforms := inspectPublishedDockerImageManifestPlatforms(t)
	requireManifestPlatform(t, observedPlatforms, "linux/amd64", manifestOutput)
	requireManifestPlatform(t, observedPlatforms, "linux/arm64", manifestOutput)
}

func TestPublishedDockerImageManifestContractMatchesDockerWorkflowTarget(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	releaseOwner := readGoreleaserGitHubOwner(t, repoRoot)
	expectedImageRepository := "ghcr.io/" + releaseOwner + "/allyourbase"
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "docker.yml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", workflowPath, err)
	}

	requireContainsAll(t, string(workflowData), []string{
		"images: " + expectedImageRepository,
	})
}

func readGoreleaserGitHubOwner(t *testing.T, repoRoot string) string {
	t.Helper()

	path := filepath.Join(repoRoot, ".goreleaser.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "github:" {
			continue
		}
		for _, candidate := range lines[i+1:] {
			if candidate == "" {
				continue
			}
			if !strings.HasPrefix(candidate, "    ") {
				break
			}
			trimmed := strings.TrimSpace(candidate)
			if strings.HasPrefix(trimmed, "owner: ") {
				return strings.TrimSpace(strings.TrimPrefix(trimmed, "owner: "))
			}
		}
	}
	t.Fatalf("missing release.github.owner in %s", path)
	return ""
}

func inspectPublishedDockerImageManifestPlatforms(t *testing.T) (string, []string) {
	t.Helper()

	cmd := exec.Command("docker", "manifest", "inspect", publishedDockerImageRef)
	output, err := cmd.CombinedOutput()
	manifestOutput := strings.TrimSpace(string(output))
	if err != nil {
		skipOrFailDockerManifestInspect(t, err, manifestOutput)
	}

	var parsedManifest dockerManifestInspectResponse
	if err := json.Unmarshal(output, &parsedManifest); err != nil {
		t.Fatalf("docker manifest inspect returned non-JSON output: %v output=%s", err, manifestOutput)
	}

	observedPlatforms := make([]string, 0, len(parsedManifest.Manifests))
	for _, manifest := range parsedManifest.Manifests {
		observedPlatforms = append(observedPlatforms, formatDockerManifestPlatform(manifest.Platform))
	}
	sort.Strings(observedPlatforms)

	return manifestOutput, observedPlatforms
}

func skipOrFailDockerManifestInspect(t *testing.T, err error, manifestOutput string) {
	t.Helper()

	lowerOutput := strings.ToLower(manifestOutput)
	for _, signature := range retryableDockerManifestInspectSignatures {
		if strings.Contains(lowerOutput, signature) {
			t.Skipf("skipping due to transient registry failure from docker manifest inspect: %v output=%s", err, manifestOutput)
		}
	}

	if execErr := new(exec.Error); errors.As(err, &execErr) {
		t.Skipf("skipping: Docker CLI not found: %v", err)
	}
	if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
		for _, signature := range dockerManifestDaemonUnavailableSignatures {
			if strings.Contains(lowerOutput, signature) {
				t.Skipf("skipping: Docker daemon unavailable for docker manifest inspect: %s", manifestOutput)
			}
		}
	}

	t.Fatalf("docker manifest inspect failed (non-transient): %v output=%s", err, manifestOutput)
}

func requireManifestPlatform(t *testing.T, observedPlatforms []string, expectedPlatform string, manifestOutput string) {
	t.Helper()

	for _, platform := range observedPlatforms {
		if platform == expectedPlatform {
			return
		}
	}

	t.Fatalf(
		"published image manifest missing %s platform entry; observed platforms=%s output=%s",
		expectedPlatform,
		strings.Join(observedPlatforms, ", "),
		manifestOutput,
	)
}

func formatDockerManifestPlatform(platform dockerManifestPlatform) string {
	return strings.TrimSpace(platform.OS) + "/" + strings.TrimSpace(platform.Architecture)
}
