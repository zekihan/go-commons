package environment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironmentFileStatErrorStopsBeforeOverride(t *testing.T) {
	t.Chdir(t.TempDir())

	err := os.WriteFile("regular", []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// A symlink to a child of a regular file makes Stat return ENOTDIR,
	// which is not a missing file and must abort loading.
	err = os.Symlink(filepath.Join("regular", "missing"), ".env")
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err = os.WriteFile(".override.env", []byte("SHOULD_NOT=apply\n"), 0o600)
	if err != nil {
		t.Fatalf("write override: %v", err)
	}

	t.Setenv("SHOULD_NOT", "original")

	err = LoadEnvironmentFile()
	if err == nil || os.IsNotExist(err) || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("LoadEnvironmentFile() = %v", err)
	}

	if got := os.Getenv("SHOULD_NOT"); got != "original" {
		t.Fatalf("SHOULD_NOT = %q after a failed .env stat", got)
	}
}

func TestLoadEnvironmentFileParseErrorStops(t *testing.T) {
	t.Chdir(t.TempDir())

	err := os.WriteFile(".env", []byte("BROKEN=\"unterminated\n"), 0o600)
	if err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err = os.WriteFile(".override.env", []byte("LATER=changed\n"), 0o600)
	if err != nil {
		t.Fatalf("write override: %v", err)
	}

	t.Setenv("LATER", "original")
	t.Setenv("BROKEN", "original")

	err = LoadEnvironmentFile()
	if err == nil || !strings.Contains(err.Error(), "error loading .env file") {
		t.Fatalf("LoadEnvironmentFile() = %v", err)
	}

	if got := os.Getenv("LATER"); got != "original" {
		t.Fatalf("LATER = %q, override was applied after a .env parse error", got)
	}

	if got := os.Getenv("BROKEN"); got != "original" {
		t.Fatalf("BROKEN = %q, partial .env value was applied", got)
	}
}

func TestLoadEnvironmentFileSkipsEmptyPathsAndStopsOnLaterFile(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)

	err := os.WriteFile(".env", []byte("FROM_DOT=dot\n"), 0o600)
	if err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err = os.WriteFile(".override.env", []byte("FROM_OVERRIDE=override\n"), 0o600)
	if err != nil {
		t.Fatalf("write override: %v", err)
	}

	err = os.Mkdir("skipped-dir", 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = os.WriteFile("good.env", []byte("FROM_GOOD=good\n"), 0o600)
	if err != nil {
		t.Fatalf("write good: %v", err)
	}

	err = os.WriteFile("bad.env", []byte("FROM_BAD=\"unterminated\n"), 0o600)
	if err != nil {
		t.Fatalf("write bad: %v", err)
	}

	t.Setenv("FROM_DOT", "original")
	t.Setenv("FROM_OVERRIDE", "original")
	t.Setenv("FROM_GOOD", "original")
	t.Setenv("FROM_BAD", "original")

	err = LoadEnvironmentFile("", "skipped-dir", "", "good.env", "bad.env")
	if err == nil || !strings.Contains(err.Error(), "bad.env") {
		t.Fatalf("LoadEnvironmentFile() = %v", err)
	}

	if os.Getenv("FROM_DOT") != "dot" || os.Getenv("FROM_OVERRIDE") != "override" ||
		os.Getenv("FROM_GOOD") != "good" || os.Getenv("FROM_BAD") != "original" {
		t.Fatalf("env after partial load: dot=%q override=%q good=%q bad=%q",
			os.Getenv("FROM_DOT"), os.Getenv("FROM_OVERRIDE"),
			os.Getenv("FROM_GOOD"), os.Getenv("FROM_BAD"))
	}
}

func TestLoadEnvironmentFileSkipsDirectoryPlaceholders(t *testing.T) {
	t.Chdir(t.TempDir())

	err := os.Mkdir(".env", 0o750)
	if err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	err = os.Mkdir(".override.env", 0o750)
	if err != nil {
		t.Fatalf("mkdir override: %v", err)
	}

	err = os.WriteFile("extra.env", []byte("# comment\n\nFROM_EXTRA=extra\n"), 0o600)
	if err != nil {
		t.Fatalf("write extra: %v", err)
	}

	t.Setenv("FROM_EXTRA", "original")
	t.Setenv("UNRELATED", "stay")

	err = LoadEnvironmentFile("extra.env")
	if err != nil {
		t.Fatalf("LoadEnvironmentFile() = %v", err)
	}

	if os.Getenv("FROM_EXTRA") != "extra" || os.Getenv("UNRELATED") != "stay" {
		t.Fatalf("FROM_EXTRA=%q UNRELATED=%q", os.Getenv("FROM_EXTRA"), os.Getenv("UNRELATED"))
	}
}
