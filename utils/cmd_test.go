package utils

import (
	"os"
	"path/filepath"
	"testing"
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
