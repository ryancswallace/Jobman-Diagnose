// Package portablepath validates path values embedded in persisted documents.
package portablepath

import (
	"path"
	"path/filepath"
	"strings"
)

// IsCleanAbsolute reports whether value is a clean absolute path in either
// POSIX or Windows syntax. Persisted evidence may be validated on an operating
// system other than the one that collected it, so filepath.IsAbs alone is not
// sufficient here.
func IsCleanAbsolute(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	slashPath := strings.ReplaceAll(value, `\`, "/")
	if strings.HasPrefix(slashPath, "//") {
		return isCleanUNCPath(slashPath)
	}
	if filepath.IsAbs(value) && filepath.Clean(value) == value {
		return true
	}
	if strings.HasPrefix(value, "/") {
		return path.Clean(value) == value
	}

	if isWindowsDriveAbsolute(slashPath) {
		return path.Clean(slashPath) == slashPath
	}

	return false
}

func isWindowsDriveAbsolute(value string) bool {
	if len(value) < 3 || value[1] != ':' || value[2] != '/' {
		return false
	}
	letter := value[0]

	return letter >= 'A' && letter <= 'Z' || letter >= 'a' && letter <= 'z'
}

func isCleanUNCPath(value string) bool {
	remainder := strings.TrimPrefix(value, "//")
	parts := strings.Split(remainder, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return false
	}

	return path.Clean("/"+remainder) == "/"+remainder
}
