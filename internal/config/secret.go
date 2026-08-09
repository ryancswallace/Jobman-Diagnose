package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maximumSecretBytes = 64 * 1024

// ResolveCredential reads one explicitly referenced credential. The caller
// owns the returned bytes and must never include them in an error or report.
//
//nolint:cyclop // Environment and private-file sources share one bounded secret-resolution boundary.
func ResolveCredential(reference *SecretReference, getenv func(string) (string, bool)) ([]byte, error) {
	if reference == nil {
		return nil, nil
	}
	if err := validateSecretReference(reference); err != nil {
		return nil, err
	}
	if reference.Environment != "" {
		if getenv == nil {
			return nil, errors.New("resolve provider credential: environment resolver is unavailable")
		}
		value, ok := getenv(reference.Environment)
		if !ok || value == "" {
			return nil, errors.New("resolve provider credential: referenced environment value is unavailable")
		}
		if len(value) > maximumSecretBytes {
			return nil, fmt.Errorf("resolve provider credential: value exceeds %d bytes", maximumSecretBytes)
		}

		return []byte(value), nil
	}
	info, err := os.Lstat(reference.File)
	if err != nil {
		return nil, fmt.Errorf("resolve provider credential: inspect private file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maximumSecretBytes {
		return nil, errors.New("resolve provider credential: file must be private, regular, and bounded")
	}
	// #nosec G304 -- the path was an explicit validated secret reference.
	file, err := os.Open(reference.File)
	if err != nil {
		return nil, fmt.Errorf("resolve provider credential: open private file: %w", err)
	}
	value, readErr := io.ReadAll(io.LimitReader(file, maximumSecretBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("resolve provider credential: read private file: %w", err)
	}
	if len(value) > maximumSecretBytes {
		return nil, fmt.Errorf("resolve provider credential: value exceeds %d bytes", maximumSecretBytes)
	}
	value = []byte(strings.TrimSuffix(string(value), "\n"))
	if len(value) == 0 {
		return nil, errors.New("resolve provider credential: referenced value is empty")
	}

	return value, nil
}
