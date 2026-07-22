package testutil

import (
	"reflect"
	"testing"
)

func TestMinIOContainerArgsUsePinnedImageAndIsolatedState(t *testing.T) {
	t.Parallel()

	options := MinIOHarnessOptions{
		ContainerName: "ayb-testpg-run-one",
		Bucket:        "ayb-testpg-run-one",
	}

	got := minIOContainerArgs(options, 19000, 2)
	want := []string{
		"run", "-d", "--name", "ayb-testpg-run-one-2",
		"-p", "127.0.0.1:19000:9000",
		"-e", "MINIO_ROOT_USER=aybminio",
		"-e", "MINIO_ROOT_PASSWORD=aybminiosecret",
		"minio/minio:RELEASE.2025-09-07T16-13-09Z",
		"server", "/data", "--address", ":9000",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minIOContainerArgs() = %#v, want %#v", got, want)
	}
}

func TestMinIOCleanupArgsTargetOnlyStartedContainer(t *testing.T) {
	t.Parallel()

	first := minIOCleanupArgs("container-id-one")
	second := minIOCleanupArgs("container-id-two")
	if !reflect.DeepEqual(first, []string{"rm", "-f", "container-id-one"}) {
		t.Fatalf("first cleanup args = %#v", first)
	}
	if !reflect.DeepEqual(second, []string{"rm", "-f", "container-id-two"}) {
		t.Fatalf("second cleanup args = %#v", second)
	}
}
