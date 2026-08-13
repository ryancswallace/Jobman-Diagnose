//go:build ignore

package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	const value = "12O4"
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		fmt.Fprintf(os.Stderr, "transform record partner-west:8841: %v\n", err)
		os.Exit(1)
	}
}
