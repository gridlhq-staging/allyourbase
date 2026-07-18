//go:build cell

package celltopology

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeDownArgsRemovesOwnedImages(t *testing.T) {
	baseArgs := []string{"-f", "compose.yml", "-p", "owned-project"}
	want := []string{"-f", "compose.yml", "-p", "owned-project", "down", "-v", "--rmi", "local", "--remove-orphans"}
	if got := composeDownArgs(baseArgs); !reflect.DeepEqual(got, want) {
		t.Fatalf("compose down args = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(baseArgs, []string{"-f", "compose.yml", "-p", "owned-project"}) {
		t.Fatalf("compose down args mutated caller prefix: %q", baseArgs)
	}
}

func TestCellDataUsesTmpfs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(packageDir(t), "testdata", "compose.override.yml"))
	if err != nil {
		t.Fatalf("read cell compose override: %v", err)
	}
	var override struct {
		Volumes map[string]struct {
			DriverOpts map[string]string `yaml:"driver_opts"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(raw, &override); err != nil {
		t.Fatalf("parse cell compose override: %v", err)
	}

	for _, volumeName := range []string{"minio-data", "postgres-data"} {
		driverOpts := override.Volumes[volumeName].DriverOpts
		if driverOpts["type"] != "tmpfs" || driverOpts["device"] != "tmpfs" {
			t.Fatalf("%s driver options = %#v, want tmpfs type and device", volumeName, driverOpts)
		}
		if !strings.Contains(driverOpts["o"], "size=") {
			t.Fatalf("%s tmpfs options = %q, want an explicit size limit", volumeName, driverOpts["o"])
		}
	}
}
