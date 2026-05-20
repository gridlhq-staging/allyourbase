package auth

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestConfigureArgon2AffectsHashParameters(t *testing.T) {
	originalMemory := argonMemory
	originalTime := argonTime
	originalThreads := argonThreads
	defer func() {
		testutil.NoError(t, ConfigureArgon2(int(originalMemory), int(originalTime), int(originalThreads)))
	}()

	testutil.NoError(t, ConfigureArgon2(2048, 2, 3))
	hash, err := hashPassword("argon2-config-test-password")
	testutil.NoError(t, err)

	testutil.True(t, strings.Contains(hash, "m=2048"), "hash should encode configured memory")
	testutil.True(t, strings.Contains(hash, "t=2"), "hash should encode configured time")
	testutil.True(t, strings.Contains(hash, "p=3"), "hash should encode configured threads")
}

func TestConfigureArgon2ConcurrentHashesRemainVerifiable(t *testing.T) {
	originalMemory := argonMemory
	originalTime := argonTime
	originalThreads := argonThreads
	defer func() {
		testutil.NoError(t, ConfigureArgon2(int(originalMemory), int(originalTime), int(originalThreads)))
	}()

	configs := []struct {
		memory  uint32
		time    uint32
		threads uint8
	}{
		{memory: 1024, time: 1, threads: 1},
		{memory: 2048, time: 2, threads: 2},
	}

	const workers = 4
	const iterations = 50

	start := make(chan struct{})
	stop := make(chan struct{})
	errCh := make(chan error, workers*iterations)
	var configWG sync.WaitGroup
	configWG.Add(1)
	go func() {
		defer configWG.Done()
		<-start
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				cfg := configs[i%len(configs)]
				if err := ConfigureArgon2(int(cfg.memory), int(cfg.time), int(cfg.threads)); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()

	var hashWG sync.WaitGroup
	close(start)
	for worker := 0; worker < workers; worker++ {
		hashWG.Add(1)
		go func() {
			defer hashWG.Done()
			for i := 0; i < iterations; i++ {
				hash, err := hashPassword("argon2-concurrency-password")
				if err != nil {
					errCh <- err
					return
				}
				ok, err := verifyPassword(hash, "argon2-concurrency-password")
				if err != nil {
					errCh <- err
					return
				}
				if !ok {
					errCh <- errors.New("configured hash did not verify")
					return
				}
			}
		}()
	}

	hashWG.Wait()
	close(stop)
	configWG.Wait()
	close(errCh)

	for err := range errCh {
		testutil.NoError(t, err)
	}
}

func TestConfigureArgon2RejectsInvalidParametersWithoutChangingState(t *testing.T) {
	originalMemory, originalTime, originalThreads := currentArgon2Config()

	tests := []struct {
		name    string
		memory  int
		time    int
		threads int
		wantErr string
	}{
		{name: "memory too low", memory: 512, time: 1, threads: 1, wantErr: "argon2 memory must be at least 1024 KiB"},
		{name: "time too low", memory: 1024, time: 0, threads: 1, wantErr: "argon2 time must be at least 1"},
		{name: "threads too low", memory: 1024, time: 1, threads: 0, wantErr: "argon2 threads must be between 1 and 255"},
		{name: "threads too high", memory: 1024, time: 1, threads: 256, wantErr: "argon2 threads must be between 1 and 255"},
		{name: "negative memory", memory: -1, time: 1, threads: 1, wantErr: "argon2 memory must be at least 1024 KiB"},
		{name: "negative time", memory: 1024, time: -1, threads: 1, wantErr: "argon2 time must be at least 1"},
		{name: "negative threads", memory: 1024, time: 1, threads: -1, wantErr: "argon2 threads must be between 1 and 255"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfigureArgon2(tt.memory, tt.time, tt.threads)
			testutil.ErrorContains(t, err, tt.wantErr)

			memory, time, threads := currentArgon2Config()
			testutil.Equal(t, originalMemory, memory)
			testutil.Equal(t, originalTime, time)
			testutil.Equal(t, originalThreads, threads)
		})
	}
}
