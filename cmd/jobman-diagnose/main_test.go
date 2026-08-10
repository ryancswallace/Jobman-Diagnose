package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecuteMapsOutputAndExitStatus(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if status := execute([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); status != 0 ||
		!strings.Contains(stdout.String(), "jobman-diagnose") {
		t.Fatalf("execute(version) = %d, stdout %q, stderr %q", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := execute([]string{"--unknown"}, strings.NewReader(""), &stdout, &stderr); status != 2 || stderr.Len() == 0 {
		t.Fatalf("execute(invalid) = %d, stderr %q", status, stderr.String())
	}
}
