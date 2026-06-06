package utils

import (
	"context"
	"os"
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
