// Command evaluationfixtures regenerates synthetic, nonsecret evaluation evidence.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

type fixtureSpec struct {
	FailureClass string
	Stderr       string
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stderr))
}

func execute(arguments []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("evaluationfixtures", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "testdata/evaluation/evidence", "fixture output directory")
	manifest := flags.String("manifest", "", "optional generated corpus manifest path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		if _, err := fmt.Fprintln(stderr, "evaluationfixtures does not accept positional arguments"); err != nil {
			return 2
		}
		return 2
	}
	if err := generate(*output); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "generate evaluation fixtures: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if *manifest != "" {
		if err := writeCorpus(*manifest); err != nil {
			if _, writeErr := fmt.Fprintf(stderr, "generate evaluation corpus: %v\n", err); writeErr != nil {
				return 1
			}
			return 1
		}
	}
	return 0
}

func generate(output string) error {
	fixtures := evaluationFixtures()
	if err := os.MkdirAll(output, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		specification := fixtures[name]
		failureClass := specification.FailureClass
		if failureClass == "" {
			failureClass = "nonzero_exit"
		}
		evidence, err := testevidence.Failed(failureClass, []byte(specification.Stderr))
		if err != nil {
			return fmt.Errorf("construct %s: %w", name, err)
		}
		evidence.Source.Capabilities = append(evidence.Source.Capabilities, "configured_value_redaction_v1")
		evidence, err = diagnostic.Seal(evidence)
		if err != nil {
			return fmt.Errorf("seal %s with log-redaction capability: %w", name, err)
		}
		path := filepath.Join(output, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- explicit development output.
		if err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
		encodeErr := diagnostic.Encode(file, evidence)
		closeErr := file.Close()
		if encodeErr != nil {
			return fmt.Errorf("encode %s: %w", name, encodeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", name, closeErr)
		}
	}
	return nil
}

func writeCorpus(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- explicit development output.
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(evaluationCorpus())
	closeErr := file.Close()
	if encodeErr != nil {
		return fmt.Errorf("encode manifest: %w", encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close manifest: %w", closeErr)
	}

	return nil
}
