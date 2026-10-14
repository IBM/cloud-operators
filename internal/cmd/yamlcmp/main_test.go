package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsEquivalentYAMLFormatting(t *testing.T) {
	left := writeTestFile(t, "left.yaml", "items:\n- name: example\n  description: 'two words'\n")
	right := writeTestFile(t, "right.yaml", "items:\n  - name: example\n    description: two words\n")

	if err := run([]string{left, right}); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsDifferentYAMLValues(t *testing.T) {
	left := writeTestFile(t, "left.yaml", "value: one\n")
	right := writeTestFile(t, "right.yaml", "value: two\n")

	err := run([]string{left, right})
	if err == nil || !strings.Contains(err.Error(), "contain different YAML values") {
		t.Fatalf("expected different-values error, got %v", err)
	}
}

func writeTestFile(t *testing.T, name, contents string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return filename
}
