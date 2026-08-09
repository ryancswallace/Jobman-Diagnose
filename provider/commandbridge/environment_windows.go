//go:build windows

package commandbridge

import "os"

func platformEnvironment() []string {
	result := []string{}
	for _, name := range []string{"SYSTEMROOT", "WINDIR", "TEMP", "TMP"} {
		if value, ok := os.LookupEnv(name); ok {
			result = append(result, name+"="+value)
		}
	}

	return result
}
