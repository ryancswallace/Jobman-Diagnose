package portablepath

import "testing"

func TestIsCleanAbsolute(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"/srv/main.go":              true,
		`C:\srv\main.go`:            true,
		"C:/srv/main.go":            true,
		`\\server\share\main.go`:    true,
		"relative/main.go":          false,
		"/srv/../main.go":           false,
		`C:\srv\..\main.go`:         false,
		`\\server\share\..\main.go`: false,
		"C:relative/main.go":        false,
		`\\server`:                  false,
	}
	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if actual := IsCleanAbsolute(value); actual != expected {
				t.Fatalf("IsCleanAbsolute(%q) = %t, want %t", value, actual, expected)
			}
		})
	}
}
