// Command jobman-diagnose diagnoses bounded evidence exported by Jobman.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ryancswallace/jobman-diagnose/internal/cli"
)

func main() {
	os.Exit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func execute(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	err := cli.Run(arguments, stdin, stdout, stderr)
	if err != nil {
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
	}
	return cli.ExitCode(err)
}
