//go:build ignore

package main

import (
	"errors"
	"fmt"
	"os"
)

func validateConfiguration() error {
	return errors.New("validate database.dsn: missing setting")
}

func loadConfiguration() error {
	if err := validateConfiguration(); err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	return nil
}

func main() {
	if err := loadConfiguration(); err != nil {
		fmt.Fprintf(os.Stderr, "start worker: %v\n", err)
		os.Exit(1)
	}
}
