package config

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/pelletier/go-toml/v2"
)

func TestAuthArgon2DefaultRoundTrip(t *testing.T) {
	cfg := Default()
	testutil.Equal(t, 65536, cfg.Auth.Argon2Memory)
	testutil.Equal(t, 3, cfg.Auth.Argon2Time)
	testutil.Equal(t, 2, cfg.Auth.Argon2Threads)

	encoded, err := toml.Marshal(cfg)
	testutil.NoError(t, err)

	decoded := Default()
	testutil.NoError(t, toml.Unmarshal(encoded, decoded))
	testutil.Equal(t, cfg.Auth.Argon2Memory, decoded.Auth.Argon2Memory)
	testutil.Equal(t, cfg.Auth.Argon2Time, decoded.Auth.Argon2Time)
	testutil.Equal(t, cfg.Auth.Argon2Threads, decoded.Auth.Argon2Threads)
}

func TestApplyAuthArgon2EnvVars(t *testing.T) {
	t.Setenv("AYB_AUTH_ARGON2_MEMORY", "131072")
	t.Setenv("AYB_AUTH_ARGON2_TIME", "4")
	t.Setenv("AYB_AUTH_ARGON2_THREADS", "3")

	cfg := Default()
	testutil.NoError(t, applyEnv(cfg))

	testutil.Equal(t, 131072, cfg.Auth.Argon2Memory)
	testutil.Equal(t, 4, cfg.Auth.Argon2Time)
	testutil.Equal(t, 3, cfg.Auth.Argon2Threads)
}

func TestValidateAuthArgon2RejectsOutOfRange(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "memory too low",
			mutate: func(cfg *Config) {
				cfg.Auth.Argon2Memory = 512
			},
			wantErr: "auth.argon2_memory must be at least 1024",
		},
		{
			name: "time too low",
			mutate: func(cfg *Config) {
				cfg.Auth.Argon2Time = 0
			},
			wantErr: "auth.argon2_time must be at least 1",
		},
		{
			name: "threads too low",
			mutate: func(cfg *Config) {
				cfg.Auth.Argon2Threads = 0
			},
			wantErr: "auth.argon2_threads must be between 1 and 255",
		},
		{
			name: "threads too high",
			mutate: func(cfg *Config) {
				cfg.Auth.Argon2Threads = 256
			},
			wantErr: "auth.argon2_threads must be between 1 and 255",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(cfg)
			err := cfg.Validate()
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}
