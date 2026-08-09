// Command jobman-diagnose diagnoses bounded evidence exported by Jobman.
package main

import (
	"fmt"
	"os"

	"github.com/ryancswallace/jobman-diagnose/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
