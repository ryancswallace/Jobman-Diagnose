package cli

import (
	"fmt"
	"io"
	"strings"
)

type colorMode string

const (
	colorAuto   colorMode = "auto"
	colorAlways colorMode = "always"
	colorNever  colorMode = "never"
)

type colorModeValue colorMode

func (value *colorModeValue) Set(encoded string) error {
	mode := colorMode(strings.ToLower(strings.TrimSpace(encoded)))
	switch mode {
	case colorAuto, colorAlways, colorNever:
		*value = colorModeValue(mode)
		return nil
	default:
		return fmt.Errorf("must be auto, always, or never")
	}
}

func (value *colorModeValue) String() string {
	if value == nil {
		return ""
	}

	return string(*value)
}

func colorEnabled(mode colorMode, destination io.Writer, environment runtimeEnvironment) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	case colorAuto:
		if environment.lookupEnv != nil {
			if noColor, present := environment.lookupEnv("NO_COLOR"); present && noColor != "" {
				return false
			}
			if term, _ := environment.lookupEnv("TERM"); strings.EqualFold(strings.TrimSpace(term), "dumb") {
				return false
			}
		}
		return environment.interactive != nil && environment.interactive(destination)
	default:
		return false
	}
}
