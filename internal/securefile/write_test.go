package securefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteAtomicCreatesPrivateFileWithoutOverwrite(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "report.json")
	if err := WriteAtomic(destination, func(writer io.Writer) error {
		_, err := writer.Write([]byte("first"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- destination is a test-owned path under t.TempDir.
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "first" || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("contents/mode = %q / %o", contents, info.Mode().Perm())
	}
	if writeErr := WriteAtomic(destination, func(writer io.Writer) error {
		_, writeErr := writer.Write([]byte("second"))
		return writeErr
	}); !errors.Is(writeErr, os.ErrExist) {
		t.Fatalf("overwrite error = %v", writeErr)
	}
	// #nosec G304 -- destination is a test-owned path under t.TempDir.
	contents, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "first" {
		t.Fatalf("existing destination changed to %q", contents)
	}
}

func TestWriteAtomicRejectsInvalidInputsAndCleansUp(t *testing.T) {
	t.Parallel()

	for _, destination := range []string{"", "-"} {
		if err := WriteAtomic(destination, func(io.Writer) error { return nil }); err == nil {
			t.Fatalf("WriteAtomic(%q) error = nil", destination)
		}
	}
	if err := WriteAtomic(filepath.Join(t.TempDir(), "nil"), nil); err == nil {
		t.Fatal("WriteAtomic(nil callback) error = nil")
	}
	if err := WriteAtomic(filepath.Join(t.TempDir(), "missing", "report"), func(io.Writer) error { return nil }); err == nil {
		t.Fatal("WriteAtomic(missing parent) error = nil")
	}

	directory := t.TempDir()
	destination := filepath.Join(directory, "report")
	sentinel := errors.New("callback failed")
	err := WriteAtomic(destination, func(writer io.Writer) error {
		if _, writeErr := writer.Write([]byte("partial")); writeErr != nil {
			return writeErr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "partial") {
		t.Fatalf("callback error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after callback failure: %v", statErr)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestWriteAtomicReplacePreservesPrivacyAndExistingFileOnFailure(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	destination := filepath.Join(directory, "evaluation.json")
	// #nosec G306 -- the fixture is deliberately public to verify that replacement restores privacy.
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicReplace(destination, func(writer io.Writer) error {
		_, err := writer.Write([]byte("new"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination) // #nosec G304 -- test-owned path.
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("replacement contents/mode = %q / %o", contents, info.Mode().Perm())
	}
	sentinel := errors.New("replacement failed")
	if replaceErr := WriteAtomicReplace(destination, func(writer io.Writer) error {
		if _, writeErr := writer.Write([]byte("partial")); writeErr != nil {
			return writeErr
		}
		return sentinel
	}); !errors.Is(replaceErr, sentinel) {
		t.Fatalf("replacement callback error = %v", replaceErr)
	}
	contents, err = os.ReadFile(destination) // #nosec G304 -- test-owned path.
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("failed replacement changed destination to %q", contents)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "evaluation.json" {
		t.Fatalf("replacement left temporary files: %v", entries)
	}
}
