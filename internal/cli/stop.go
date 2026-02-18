package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the AYB server",
	Long:  `Stop a running AllYourBase server gracefully.`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	out := cmd.OutOrStdout()

	pid, _, err := readAYBPID()
	if err != nil {
		if os.IsNotExist(err) {
			if jsonOut {
				json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "no AYB server is running"})
				return nil
			}
			fmt.Fprintln(out, "No AYB server is running (no PID file found).")
			return nil
		}
		return fmt.Errorf("reading PID file: %w", err)
	}

	// Check if process is alive.
	proc, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist — clean up stale PID file.
		cleanupPIDFile()
		if jsonOut {
			json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "stale PID file cleaned up"})
			return nil
		}
		fmt.Fprintln(out, "No AYB server is running (stale PID file cleaned up).")
		return nil
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		cleanupPIDFile()
		if jsonOut {
			json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "stale PID file cleaned up"})
			return nil
		}
		fmt.Fprintln(out, "No AYB server is running (stale PID file cleaned up).")
		return nil
	}

	// Send SIGTERM for graceful shutdown.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
	}

	// Show spinner while waiting for shutdown.
	isTTY := colorEnabled()
	sp := ui.NewStepSpinner(os.Stderr, !isTTY)
	sp.Start("Stopping server...")

	// Wait for process to exit (up to 10 seconds).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			cleanupPIDFile()
			sp.Done()
			if jsonOut {
				json.NewEncoder(out).Encode(map[string]any{"status": "stopped", "pid": pid})
				return nil
			}
			fmt.Fprintf(out, "AYB server (PID %d) stopped.\n", pid)
			return nil
		}
	}

	sp.Fail()
	if jsonOut {
		json.NewEncoder(out).Encode(map[string]any{"status": "timeout", "pid": pid, "message": "server did not stop within 10s"})
		return nil
	}
	fmt.Fprintf(out, "AYB server (PID %d) did not stop within 10 seconds. You may need to kill it manually.\n", pid)
	return nil
}

func cleanupPIDFile() {
	if path, err := aybPIDPath(); err == nil {
		os.Remove(path)
	}
}
