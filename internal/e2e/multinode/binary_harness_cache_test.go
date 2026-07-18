//go:build multinode

package multinode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type aybBinaryBuild func(moduleRoot, binPath string) ([]byte, error)

type aybBinaryCache struct {
	once    sync.Once
	rootDir string
	binPath string
	output  []byte
	err     error
}

var sharedAYBBinary aybBinaryCache

func (cache *aybBinaryCache) build(moduleRoot string, build aybBinaryBuild) (string, error) {
	cache.once.Do(func() {
		cache.rootDir, cache.err = os.MkdirTemp("", "ayb-multinode-binary-*")
		if cache.err != nil {
			return
		}
		cache.binPath = filepath.Join(cache.rootDir, "ayb")
		cache.output, cache.err = build(moduleRoot, cache.binPath)
	})
	return cache.binPath, cache.err
}

func (cache *aybBinaryCache) cleanup() error {
	if cache.rootDir == "" {
		return nil
	}
	return os.RemoveAll(cache.rootDir)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := sharedAYBBinary.cleanup(); err != nil {
		fmt.Fprintf(os.Stderr, "remove shared multinode AYB binary: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func buildAYBBinary(t *testing.T) string {
	t.Helper()

	binPath, err := sharedAYBBinary.build(moduleRoot(t), runAYBBinaryBuild)
	if err != nil {
		t.Fatalf("build ayb binary: %v\n%s", err, strings.TrimSpace(string(sharedAYBBinary.output)))
	}
	return binPath
}

func runAYBBinaryBuild(moduleRoot, binPath string) ([]byte, error) {
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ayb") //nolint:gosec
	cmd.Dir = moduleRoot
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func TestAYBBinaryCacheBuildsOnceAndCleansUp(t *testing.T) {
	var cache aybBinaryCache
	buildCalls := 0
	build := func(_ string, binPath string) ([]byte, error) {
		buildCalls++
		return nil, os.WriteFile(binPath, []byte("test binary"), 0o755)
	}

	firstPath, err := cache.build(moduleRoot(t), build)
	if err != nil {
		t.Fatalf("first cached build: %v", err)
	}
	secondPath, err := cache.build(moduleRoot(t), build)
	if err != nil {
		t.Fatalf("second cached build: %v", err)
	}
	if buildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", buildCalls)
	}
	if secondPath != firstPath {
		t.Fatalf("cached path = %q, want %q", secondPath, firstPath)
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("cached binary missing before cleanup: %v", err)
	}

	rootDir := filepath.Dir(firstPath)
	if err := cache.cleanup(); err != nil {
		t.Fatalf("cleanup cached binary: %v", err)
	}
	if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Fatalf("cache root after cleanup: err=%v, want not-exist", err)
	}
}
