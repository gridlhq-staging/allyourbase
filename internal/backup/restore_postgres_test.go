package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestRecoveryInstanceStartStopLifecycle(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	var calls [][]string
	inst.runCommand = func(ctx context.Context, name string, args ...string) error {
		_ = ctx
		line := append([]string{name}, args...)
		calls = append(calls, line)
		return nil
	}

	if err := inst.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inst.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	wantStart := []string{"pg_ctl", "start", "-D", "/tmp/restore-data", "-o", "-p 55432", "-w", "-t", "300"}
	wantStop := []string{"pg_ctl", "stop", "-D", "/tmp/restore-data", "-m", "fast"}
	if len(calls) != 2 {
		t.Fatalf("calls = %d; want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[0], wantStart) {
		t.Fatalf("start args = %#v; want %#v", calls[0], wantStart)
	}
	if !reflect.DeepEqual(calls[1], wantStop) {
		t.Fatalf("stop args = %#v; want %#v", calls[1], wantStop)
	}
	if got := inst.ConnURL(); got != "postgresql://localhost:55432/postgres" {
		t.Fatalf("ConnURL = %q", got)
	}
}

func TestRecoveryInstanceUsePrimaryConnectionURL(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	err := inst.UsePrimaryConnectionURL("postgresql://ayb:secret@db.internal:6432/app?sslmode=disable&application_name=restore")
	if err != nil {
		t.Fatalf("UsePrimaryConnectionURL: %v", err)
	}

	want := "postgresql://ayb:secret@127.0.0.1:55432/app?sslmode=disable&application_name=restore"
	if got := inst.ConnURL(); got != want {
		t.Fatalf("ConnURL = %q; want %q", got, want)
	}
}

func TestRecoveryInstanceUsePrimaryConnectionURLDefaultsDatabase(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	if err := inst.UsePrimaryConnectionURL("postgresql://ayb:secret@db.internal:6432"); err != nil {
		t.Fatalf("UsePrimaryConnectionURL: %v", err)
	}

	want := "postgresql://ayb:secret@127.0.0.1:55432/postgres"
	if got := inst.ConnURL(); got != want {
		t.Fatalf("ConnURL = %q; want %q", got, want)
	}
}

func TestRecoveryInstanceWaitForRecoveryTimeout(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	inst.waitTimeout = 30 * time.Millisecond
	inst.pollInterval = 5 * time.Millisecond
	inst.queryRecoveryFn = func(ctx context.Context, connURL string) (bool, error) {
		_ = ctx
		_ = connURL
		return true, nil
	}

	err := inst.WaitForRecovery(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRecoveryInstanceWaitForRecoverySuccess(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	inst.waitTimeout = time.Second
	inst.pollInterval = 5 * time.Millisecond
	attempts := 0
	inst.queryRecoveryFn = func(ctx context.Context, connURL string) (bool, error) {
		_ = ctx
		_ = connURL
		attempts++
		return attempts < 2, nil
	}

	if err := inst.WaitForRecovery(context.Background()); err != nil {
		t.Fatalf("WaitForRecovery: %v", err)
	}
}

func TestRecoveryInstanceFindFreePort(t *testing.T) {
	t.Parallel()

	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("FindFreePort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("invalid port: %d", port)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("expected to bind to returned port %d: %v", port, err)
	}
	_ = ln.Close()
}

// realPGCtlAddressInUseOutput is the diagnostic pg_ctl surfaces when the
// postmaster loses the race for its listen socket. The errno lives only in the
// captured server log: pg_ctl exits with a plain status code, so exec.ExitError
// carries no syscall.Errno to unwrap.
const realPGCtlAddressInUseOutput = `waiting for server to start....` +
	`2026-07-30 12:00:00.000 UTC [41234] LOG:  could not bind IPv4 address "127.0.0.1": Address already in use` + "\n" +
	`2026-07-30 12:00:00.000 UTC [41234] HINT:  Is another postmaster already running on port 55432? If not, wait a few seconds and retry.` + "\n" +
	`2026-07-30 12:00:00.000 UTC [41234] WARNING:  could not create listen socket for "127.0.0.1"` + "\n" +
	`2026-07-30 12:00:00.000 UTC [41234] FATAL:  could not create any TCP/IP sockets` + "\n" +
	` stopped waiting` + "\n" +
	`pg_ctl: could not start server` + "\n" +
	`Examine the log output.`

func TestIsAddressInUse(t *testing.T) {
	t.Parallel()

	pgCtlExitErr := fmt.Errorf(
		"command pg_ctl [start -D /tmp/restore-data -o -p 55432 -w -t 300] failed: %w (output: %s)",
		&exec.ExitError{ProcessState: &os.ProcessState{}}, realPGCtlAddressInUseOutput,
	)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "typed errno unwraps to EADDRINUSE",
			err:  fmt.Errorf("starting recovery postgres instance: %w", &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}),
			want: true,
		},
		{
			name: "bare EADDRINUSE",
			err:  syscall.EADDRINUSE,
			want: true,
		},
		{
			name: "real pg_ctl address-in-use diagnostic without a typed errno",
			err:  pgCtlExitErr,
			want: true,
		},
		{
			name: "IPv6 bind failure diagnostic",
			err:  fmt.Errorf(`LOG:  could not bind IPv6 address "::1": Address already in use`),
			want: true,
		},
		{
			name: "unrelated pg_ctl failure",
			err: fmt.Errorf(
				"command pg_ctl [start -D /tmp/restore-data] failed: %w (output: %s)",
				&exec.ExitError{ProcessState: &os.ProcessState{}},
				"FATAL:  data directory \"/tmp/restore-data\" has invalid permissions\npg_ctl: could not start server",
			),
			want: false,
		},
		{
			name: "unrelated errno",
			err:  fmt.Errorf("starting recovery postgres instance: %w", syscall.ECONNREFUSED),
			want: false,
		},
		{
			name: "prose mentioning address reuse without a bind failure",
			err:  errors.New("restore aborted: the operator said the address already in use policy applies"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isAddressInUse(tc.err); got != tc.want {
				t.Fatalf("isAddressInUse(%v) = %t; want %t", tc.err, got, tc.want)
			}
		})
	}
}
