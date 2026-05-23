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
