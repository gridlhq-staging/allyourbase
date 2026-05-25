package codehealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
