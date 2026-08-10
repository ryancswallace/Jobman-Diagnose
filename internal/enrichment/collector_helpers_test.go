package enrichment

import "testing"

func TestCompilerLocationHelpersRejectMalformedCoordinates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "file.go:12:7: error", want: true},
		{value: "no colon"},
		{value: "file.go:12:error"},
		{value: "file.go:x:7: error"},
		{value: "file.go:12:x: error"},
	} {
		if got := hasLineAndColumn([]byte(test.value)); got != test.want {
			t.Fatalf("hasLineAndColumn(%q) = %t, want %t", test.value, got, test.want)
		}
	}
	if digits(nil) || digits([]byte("12x")) || !digits([]byte("120")) {
		t.Fatal("digits validation changed")
	}
}
