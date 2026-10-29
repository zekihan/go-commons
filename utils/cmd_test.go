package utils

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCmd_Success(t *testing.T) {
	err := RunCmd(t.Context(), "echo", nil, "hello")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunCmd_Failure(t *testing.T) {
	err := RunCmd(t.Context(), "nonexistent-cmd", nil)
	if err == nil {
		t.Errorf("expected error for nonexistent command, got nil")
	}
}

func TestRunCmdInPath_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	err := os.WriteFile(testFile, []byte("data"), 0o600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Use 'ls' to list the file in the temp directory
	err = RunCmdInPath(t.Context(), tmpDir, "ls", nil, "test.txt")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunCmdInPath_Failure(t *testing.T) {
	tmpDir := t.TempDir()

	err := RunCmdInPath(t.Context(), tmpDir, "nonexistent-cmd", nil)
	if err == nil {
		t.Errorf("expected error for nonexistent command, got nil")
	}
}

func TestRunCmd_LargeOutputNoDeadlock(t *testing.T) {
	const timeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	// Emit far more than the 64KB OS pipe buffer on stdout while also writing to
	// stderr. The previous implementation read stderr to completion before
	// stdout and could deadlock here; this must complete promptly.
	err := RunCmd(ctx, "sh", nil, "-c", "head -c 200000 /dev/zero; echo done 1>&2")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestRunCmdCancellationHelper runs only in the subprocess used by the cancellation test.
func TestRunCmdCancellationHelper(t *testing.T) {
	address := os.Getenv("GO_COMMONS_CMD_CANCEL_ADDRESS")
	if address == "" {
		return
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}

	connection, err := dialer.DialContext(
		t.Context(),
		"tcp",
		address,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	// Keep the process running until CommandContext kills it. Accepting this
	// connection proves cancellation happens after the subprocess has started.
	var data [1]byte

	_, _ = connection.Read(data[:])
}

func TestRunCmd_Cancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, inPath := range []bool{false, true} {
		name := "RunCmd"
		if inPath {
			name = "RunCmdInPath"
		}

		t.Run(name, func(t *testing.T) {
			var config net.ListenConfig

			listener, listenErr := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if listenErr != nil {
				t.Fatal(listenErr)
			}
			defer listener.Close()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			// Close the listener if the subprocess never connects, bounding a broken test.
			timeout := time.AfterFunc(5*time.Second, func() {
				cancel()

				_ = listener.Close()
			})
			defer timeout.Stop()

			env := append(os.Environ(), "GO_COMMONS_CMD_CANCEL_ADDRESS="+listener.Addr().String())
			result := make(chan error, 1)
			directory := t.TempDir()

			go func() {
				args := []string{"-test.run=^TestRunCmdCancellationHelper$"}
				if inPath {
					result <- RunCmdInPath(ctx, directory, executable, env, args...)
				} else {
					result <- RunCmd(ctx, executable, env, args...)
				}
			}()

			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				t.Fatal(acceptErr)
			}
			defer connection.Close()

			cancel()

			commandErr := <-result
			if !errors.Is(commandErr, context.Canceled) {
				t.Errorf("expected context cancellation, got %v", commandErr)
			}

			if _, ok := errors.AsType[*exec.ExitError](commandErr); !ok {
				t.Errorf("expected subprocess exit error to remain available, got %v", commandErr)
			}
		})
	}
}
