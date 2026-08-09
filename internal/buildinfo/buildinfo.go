// Package buildinfo contains companion release metadata injected at build time.
package buildinfo

// Values are replaced by release builds. Source builds remain identifiable.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)
