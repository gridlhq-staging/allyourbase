package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDemoServerStartEnvPinsServerPort(t *testing.T) {
	baseURL := "http://127.0.0.1:49152"

	env := demoServerStartEnv("demo-secret-that-is-at-least-32-bytes", "movies", demoServerPort(baseURL))
	values := demoStartEnvValues(env)

	testutil.Equal(t, "true", values["AYB_AUTH_ENABLED"])
	testutil.Equal(t, "demo-secret-that-is-at-least-32-bytes", values["AYB_AUTH_JWT_SECRET"])
	testutil.Equal(t, "true", values["AYB_AUTH_ANONYMOUS_AUTH_ENABLED"])
	testutil.Equal(t, demoServerPort(baseURL), values["AYB_SERVER_PORT"])
	testutil.Equal(t, "http://localhost:5177", values["AYB_SERVER_SITE_URL"])
}

func TestServerURLUsesConfiguredPortBeforePIDFile(t *testing.T) {
	homeDir := t.TempDir()
	aybDir := filepath.Join(homeDir, ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))
	testutil.NoError(t, os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("12345\n3000\n"), 0o600))
	t.Setenv("HOME", homeDir)
	t.Setenv("AYB_SERVER_PORT", "49152")

	testutil.Equal(t, "http://127.0.0.1:49152", serverURL())
}

func TestServerURLIgnoresInvalidConfiguredPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, invalid := range []string{"0", "65536", "not-a-port"} {
		t.Setenv("AYB_SERVER_PORT", invalid)
		testutil.Equal(t, "http://127.0.0.1:8090", serverURL())
	}
}

func TestStartAuthEnabledDemoServerPinsPortForMovies(t *testing.T) {
	const baseURL = "http://127.0.0.1:8090"
	const jwtSecret = "movies-demo-secret-that-is-at-least-32-bytes"

	origStartCommand := demoServerStartCommandFunc
	origResolveSecret := demoResolveJWTSecretFunc
	t.Cleanup(func() {
		demoServerStartCommandFunc = origStartCommand
		demoResolveJWTSecretFunc = origResolveSecret
	})

	demoResolveJWTSecretFunc = func() (string, error) {
		return jwtSecret, nil
	}

	var capturedEnv []string
	demoServerStartCommandFunc = func(aybBin, demoName string) (*exec.Cmd, func(), error) {
		cmd, cleanup, err := demoServerStartCommand(aybBin, demoName)
		if err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(cmd.Args[3])
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		cfg, err := config.ParseTOML(data)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		testutil.Equal(t, 8092, cfg.Server.Port)

		startCmd := exec.Command("true")
		cleanupWithCapture := func() {
			capturedEnv = append([]string(nil), startCmd.Env...)
			cleanup()
		}
		return startCmd, cleanupWithCapture, nil
	}

	gotBaseURL, weStarted, err := startAuthEnabledDemoServer(baseURL, "movies")
	testutil.NoError(t, err)
	testutil.Equal(t, baseURL, gotBaseURL)
	testutil.Equal(t, true, weStarted)

	values := demoStartEnvValues(capturedEnv)
	testutil.Equal(t, jwtSecret, values["AYB_AUTH_JWT_SECRET"])
	testutil.Equal(t, "8090", values["AYB_SERVER_PORT"])
	testutil.Equal(t, "http://localhost:5177", values["AYB_SERVER_SITE_URL"])
}

func demoStartEnvValues(env []string) map[string]string {
	values := map[string]string{}
	for _, item := range env {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	return values
}
