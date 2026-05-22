package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/vector"
)

func main() {
	seedPath := "examples/movies/seed.sql"
	artifactPath := "examples/movies/embeddings.json"
	rebuildValue := os.Getenv("AYB_MOVIES_REBUILD_EMBEDDINGS")

	if !vector.ShouldRebuildMoviesEmbeddingsEnvValue(rebuildValue) {
		seedBytes, err := fs.ReadFile(examples.FS, "movies/seed.sql")
		if err != nil {
			fmt.Fprintf(os.Stderr, "movies embeddings: reading embedded seed: %v\n", err)
			os.Exit(1)
		}
		artifactBytes, err := fs.ReadFile(examples.FS, "movies/embeddings.json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "movies embeddings: reading embedded artifact: %v\n", err)
			os.Exit(1)
		}
		artifact, err := vector.LoadCommittedMoviesEmbeddingArtifact(seedBytes, artifactBytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "movies embeddings: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("loaded committed movies artifact with %d records\n", len(artifact.Records))
		return
	}

	artifact, fromCache, err := vector.LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, rebuildValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "movies embeddings: %v\n", err)
		os.Exit(1)
	}

	if fromCache {
		fmt.Printf("loaded committed movies artifact with %d records\n", len(artifact.Records))
		return
	}
	fmt.Printf("rebuilt movies artifact with %d records\n", len(artifact.Records))
}
