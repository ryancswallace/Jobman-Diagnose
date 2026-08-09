//go:build !windows

package commandbridge

func platformEnvironment() []string { return []string{"LC_ALL=C", "LANG=C"} }
