package utils

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func captureOutput(t *testing.T, f func()) string {
	t.Helper()

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	old := os.Stdout
	os.Stdout = writePipe

	t.Cleanup(func() {
		os.Stdout = old
	})

	f()

	err = writePipe.Close()
	if err != nil {
		t.Fatalf("failed to close write pipe: %v", err)
	}

	var buf bytes.Buffer

	_, err = buf.ReadFrom(readPipe)
	if err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}

	err = readPipe.Close()
	if err != nil {
		t.Fatalf("failed to close read pipe: %v", err)
	}

	return buf.String()
}

func TestDiffLines(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected []string
	}{
		{
			name: "identical",
			a:    "foo\nbar\nbaz",
			b:    "foo\nbar\nbaz",
			expected: []string{
				"  foo",
				"  bar",
				"  baz",
				"",
			},
		},
		{
			name: "addition",
			a:    "foo\nbar",
			b:    "foo\nbar\nbaz",
			expected: []string{
				"  foo",
				"  bar",
				"+ baz",
				"",
			},
		},
		{
			name: "deletion",
			a:    "foo\nbar\nbaz",
			b:    "foo\nbar",
			expected: []string{
				"  foo",
				"  bar",
				"- baz",
				"",
			},
		},
		{
			name: "change",
			a:    "foo\nbar\nbaz",
			b:    "foo\nqux\nbaz",
			expected: []string{
				"  foo",
				"- bar",
				"+ qux",
				"  baz",
				"",
			},
		},
		{
			name: "empty a",
			a:    "",
			b:    "foo\nbar",
			expected: []string{
				"+ foo",
				"+ bar",
				"",
			},
		},
		{
			name: "empty b",
			a:    "foo\nbar",
			b:    "",
			expected: []string{
				"- foo",
				"- bar",
				"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureOutput(t, func() {
				DiffLines(tt.a, tt.b)
			})

			lines := strings.Split(out, "\n")
			if len(lines) != len(tt.expected) {
				t.Fatalf("expected %d lines, got %d: %v", len(tt.expected), len(lines), lines)
			}

			for i, exp := range tt.expected {
				if lines[i] != exp {
					t.Errorf("line %d: expected %q, got %q", i, exp, lines[i])
				}
			}
		})
	}
}

func TestLongestCommonSubsequenceLines(t *testing.T) {
	tests := []struct {
		a, b     []string
		expected []string
	}{
		{
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "x", "c"},
			expected: []string{"a", "c"},
		},
		{
			a:        []string{"a", "b", "c"},
			b:        []string{"x", "y", "z"},
			expected: []string{},
		},
		{
			a:        []string{},
			b:        []string{"a", "b"},
			expected: []string{},
		},
		{
			a:        []string{"a", "b"},
			b:        []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		got := longestCommonSubsequenceLines(tt.a, tt.b)
		if len(got) != len(tt.expected) {
			t.Errorf("lcs(%v, %v): expected %v, got %v", tt.a, tt.b, tt.expected, got)

			continue
		}

		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("lcs(%v, %v): expected %v, got %v", tt.a, tt.b, tt.expected, got)

				break
			}
		}
	}
}
