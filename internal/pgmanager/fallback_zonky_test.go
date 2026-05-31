package pgmanager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/ulikunitz/xz"
)

func TestResolveLegacyArchiveSourceSelectsLatestPatch(t *testing.T) {
	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>15.12.0</version>
      <version>16.9.0</version>
      <version>16.13.0</version>
      <version>17.1.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml", r.URL.Path)
		_, _ = w.Write([]byte(metadata))
	}))
	defer srv.Close()

	source, err := resolveLegacyArchiveSource(context.Background(), "16", "darwin-arm64", srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, "embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar", source.jarFilename)
	testutil.Equal(t, srv.URL+"/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar", source.jarURL)
}

func TestEnsureBinaryFromLegacyArchiveDownloadsAndExtracts(t *testing.T) {
	txzPayload := makeLegacyTarXZ(t, map[string]string{
		"bin/postgres":               fakePostgresVersionScript("16.13"),
		"bin/initdb":                 "initdb",
		"share/postgresql/dummy.txt": "ok",
		"PG_VERSION":                 "16",
	})
	jarBytes := makeLegacyJar(t, "postgres-darwin-arm_64.txz", txzPayload)
	jarHash := sha256.Sum256(jarBytes)
	jarHashHex := hex.EncodeToString(jarHash[:])

	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>16.13.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml":
			_, _ = w.Write([]byte(metadata))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar.sha256":
			_, _ = w.Write([]byte(jarHashHex))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar":
			_, _ = w.Write(jarBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	binDir := filepath.Join(t.TempDir(), "bin")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))
	testutil.NoError(t, os.MkdirAll(cacheDir, 0o755))

	err := ensureBinaryFromLegacyArchive(context.Background(), ensureBinaryOpts{
		version:       "16",
		platform:      "darwin-arm64",
		cacheDir:      cacheDir,
		binDir:        binDir,
		legacyBaseURL: srv.URL,
	})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(binDir, "bin", "postgres"))
	testutil.NoError(t, err)
	testutil.Contains(t, string(content), "16.13")

	_, err = os.Stat(filepath.Join(cacheDir, "embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar"))
	testutil.NoError(t, err)
}

func TestExtractLegacyJarArchiveRejectsMissingPayload(t *testing.T) {
	jarBytes := makeLegacyJar(t, "README.txt", []byte("not-a-txz"))
	jarPath := filepath.Join(t.TempDir(), "missing.jar")
	testutil.NoError(t, os.WriteFile(jarPath, jarBytes, 0o644))

	err := extractLegacyJarArchive(jarPath, t.TempDir())
	testutil.ErrorContains(t, err, ".txz payload")
}

func TestEnsureBinaryReportsLegacyFallbackUsage(t *testing.T) {
	archive := makeLegacyTarXZ(t, map[string]string{
		"bin/postgres": fakePostgresVersionScript("16.9"),
		"PG_VERSION":   "16",
	})
	jarBytes := makeLegacyJar(t, "postgres-darwin-arm_64.txz", archive)
	jarHash := sha256.Sum256(jarBytes)
	jarHashHex := hex.EncodeToString(jarHash[:])

	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>16.13.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			http.NotFound(w, r)
		case "/embedded-postgres-binaries-darwin-arm64v8/maven-metadata.xml":
			_, _ = w.Write([]byte(metadata))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar.sha256":
			_, _ = w.Write([]byte(jarHashHex))
		case "/embedded-postgres-binaries-darwin-arm64v8/16.13.0/embedded-postgres-binaries-darwin-arm64v8-16.13.0.jar":
			_, _ = w.Write(jarBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:       "16",
		platform:      "darwin-arm64",
		cacheDir:      cacheDir,
		binDir:        binDir,
		sha256URL:     srv.URL + "/SHA256SUMS",
		legacyBaseURL: srv.URL,
	})
	testutil.NoError(t, err)
	testutil.True(t, usedLegacyFallback, "missing managed release assets should report legacy fallback usage")
}

func TestEnsureBinaryFallsBackWhenManagedReleaseBinaryCannotRun(t *testing.T) {
	badManagedArchive := makeTarXZWithHeaders(t, []*tar.Header{
		{Name: "ayb-postgres-16/bin/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "ayb-postgres-16/bin/postgres", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("#!/bin/sh\necho \"qemu-x86_64: Could not open '/lib64/ld-linux-x86-64.so.2': No such file or directory\" >&2\nexit 255\n"))},
		{Name: "ayb-postgres-16/lib/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "ayb-postgres-16/lib/libecpg.so.6.16", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("bad"))},
		{Name: "ayb-postgres-16/lib/libecpg.so", Typeflag: tar.TypeSymlink, Mode: 0o755, Linkname: "libecpg.so.6.16"},
		{Name: "ayb-postgres-16/PG_VERSION", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("16"))},
	}, map[string]string{
		"ayb-postgres-16/bin/postgres":        "#!/bin/sh\necho \"qemu-x86_64: Could not open '/lib64/ld-linux-x86-64.so.2': No such file or directory\" >&2\nexit 255\n",
		"ayb-postgres-16/lib/libecpg.so.6.16": "bad",
		"ayb-postgres-16/PG_VERSION":          "16",
	})
	badManagedHash := sha256.Sum256(badManagedArchive)
	badManagedHashHex := hex.EncodeToString(badManagedHash[:])

	legacyArchive := makeTarXZWithHeaders(t, []*tar.Header{
		{Name: "bin/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "bin/postgres", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(fakePostgresVersionScript("16.13")))},
		{Name: "bin/initdb", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("#!/bin/sh\necho initdb\n"))},
		{Name: "lib/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "lib/libecpg.so.6.16", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("good"))},
		{Name: "lib/libecpg.so", Typeflag: tar.TypeSymlink, Mode: 0o755, Linkname: "libecpg.so.6.16"},
		{Name: "PG_VERSION", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("16"))},
	}, map[string]string{
		"bin/postgres":        fakePostgresVersionScript("16.13"),
		"bin/initdb":          "#!/bin/sh\necho initdb\n",
		"lib/libecpg.so.6.16": "good",
		"PG_VERSION":          "16",
	})
	legacyJarBytes := makeLegacyJar(t, "postgres-darwin-arm_64.txz", legacyArchive)
	legacyJarHash := sha256.Sum256(legacyJarBytes)
	legacyJarHashHex := hex.EncodeToString(legacyJarHash[:])

	metadata := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>16.13.0</version>
    </versions>
  </versioning>
</metadata>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/managed/SHA256SUMS":
			_, _ = w.Write([]byte(badManagedHashHex + "  ayb-postgres-16-linux-arm64.tar.xz\n"))
		case "/managed/16/linux-arm64.tar.xz":
			_, _ = w.Write(badManagedArchive)
		case "/embedded-postgres-binaries-linux-arm64v8/maven-metadata.xml":
			_, _ = w.Write([]byte(metadata))
		case "/embedded-postgres-binaries-linux-arm64v8/16.13.0/embedded-postgres-binaries-linux-arm64v8-16.13.0.jar.sha256":
			_, _ = w.Write([]byte(legacyJarHashHex))
		case "/embedded-postgres-binaries-linux-arm64v8/16.13.0/embedded-postgres-binaries-linux-arm64v8-16.13.0.jar":
			_, _ = w.Write(legacyJarBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.NoError(t, os.MkdirAll(binDir, 0o755))

	usedLegacyFallback, err := ensureBinary(context.Background(), ensureBinaryOpts{
		version:       "16",
		platform:      "linux-arm64",
		cacheDir:      cacheDir,
		binDir:        binDir,
		baseURL:       srv.URL + "/managed/{version}/{platform}.tar.xz",
		sha256URL:     srv.URL + "/managed/SHA256SUMS",
		legacyBaseURL: srv.URL,
	})
	testutil.NoError(t, err)
	testutil.True(t, usedLegacyFallback, "managed release binaries that cannot execute should fall back to the legacy archive")

	versionOut, readErr := os.ReadFile(filepath.Join(binDir, "bin", "postgres"))
	testutil.NoError(t, readErr)
	testutil.Contains(t, string(versionOut), "16.13")
}

func makeLegacyJar(t *testing.T, entryName string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(entryName)
	testutil.NoError(t, err)
	_, err = w.Write(content)
	testutil.NoError(t, err)
	testutil.NoError(t, zw.Close())

	return buf.Bytes()
}

func makeLegacyTarXZ(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	testutil.NoError(t, err)

	tw := tar.NewWriter(xzw)
	for name, content := range files {
		testutil.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		testutil.NoError(t, err)
	}
	testutil.NoError(t, tw.Close())
	testutil.NoError(t, xzw.Close())
	return buf.Bytes()
}
