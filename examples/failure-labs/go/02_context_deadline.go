//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
)

func fetchSnapshot() error {
	return fmt.Errorf("GET https://inventory.internal/snapshot: %w", context.DeadlineExceeded)
}

func main() {
	if err := fetchSnapshot(); err != nil {
		fmt.Fprintf(os.Stderr, "synchronize inventory: %v\n", err)
		os.Exit(1)
	}
}
