package cli

import (
	"os"
	"os/exec"
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
