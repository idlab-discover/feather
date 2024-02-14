package util

import (
	"fmt"
	"os/exec"
)

// ExecParseError returns cleaned exit code and a message
func ExecParseError(err error) (int, string) {
	exitCode, exitMsg := 0, "gracefully"
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode, exitMsg = exitErr.ExitCode(), fmt.Sprintf("with code %d", exitCode)
		} else {
			exitCode, exitMsg = -1, fmt.Sprintf("with error %q", err.Error())
		}
	}
	return exitCode, fmt.Sprintf("exited %s", exitMsg)
}
