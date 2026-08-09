package compatibility_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const compatibleJobmanRevision = "44b90e597264534f3b64486e1d7b8ff2b5c13e15"

func TestDevelopmentWorkflowsPinCompatibleJobmanRevision(t *testing.T) {
	t.Parallel()

	workflows := []string{"codeql.yml", "fuzz.yml", "test.yml"}
	for _, name := range workflows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", "..", ".github", "workflows", name)
			refs := coordinatedJobmanRefs(t, path)
			if len(refs) == 0 {
				t.Fatalf("%s has no coordinated Jobman checkout", path)
			}
			for _, ref := range refs {
				if ref != compatibleJobmanRevision {
					t.Errorf("%s checks out Jobman %q, want %q", path, ref, compatibleJobmanRevision)
				}
			}
		})
	}
}

func coordinatedJobmanRefs(t *testing.T, path string) []string {
	t.Helper()

	// #nosec G304 -- path is a repository-owned workflow selected by the test.
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	})

	var refs []string
	scanner := bufio.NewScanner(file)
	wantRef := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "repository: ryancswallace/jobman":
			if wantRef {
				t.Fatalf("%s has a Jobman checkout without a ref", path)
			}
			wantRef = true
		case wantRef && strings.HasPrefix(line, "ref:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
			value, _, _ = strings.Cut(value, " #")
			refs = append(refs, value)
			wantRef = false
		case wantRef && (line == "" || strings.HasPrefix(line, "- name:")):
			t.Fatalf("%s has a Jobman checkout without a ref", path)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if wantRef {
		t.Fatalf("%s has a Jobman checkout without a ref", path)
	}

	return refs
}
