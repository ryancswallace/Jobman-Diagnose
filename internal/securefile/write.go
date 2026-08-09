// Package securefile writes explicit exports privately and without overwrite.
package securefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteAtomic creates destination with private permissions, atomically, and
// fails when the destination already exists.
func WriteAtomic(destination string, write func(io.Writer) error) (returned error) {
	if destination == "" || destination == "-" {
		return errors.New("write private file: destination must be a file path")
	}
	if write == nil {
		return errors.New("write private file: writer callback is nil")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("write private file: resolve destination: %w", err)
	}
	directory := filepath.Dir(absolute)
	temporary, err := os.CreateTemp(directory, ".jobman-diagnose-*.tmp")
	if err != nil {
		return fmt.Errorf("write private file: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		removeErr := os.Remove(temporaryPath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			returned = errors.Join(returned, fmt.Errorf("remove temporary file: %w", removeErr))
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("write private file: set permissions: %w", errors.Join(err, temporary.Close()))
	}
	writeErr := write(temporary)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write private file: %w", err)
	}
	if err := os.Link(temporaryPath, absolute); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("write private file: destination already exists: %w", os.ErrExist)
		}

		return fmt.Errorf("write private file: publish: %w", err)
	}

	return nil
}
