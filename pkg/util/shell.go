package util

import (
	"bytes"
	"os"
	"os/exec"
)

func ExecShellCommand(cmd string) (string, error) {
	shell := exec.Command("sh", "-c", cmd)
	stdout, err := shell.Output()
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

func ExecShellScript(script string, args ...string) error {
	// Start shell
	args = append([]string{"-s", "-x"}, args...)
	shell := exec.Command("sh", args...)
	// Pipe stdout and stderr to console
	buffer := bytes.Buffer{}
	buffer.WriteString(script)
	shell.Stdin = &buffer
	shell.Stdout = os.Stdout
	shell.Stderr = os.Stderr
	// Run command
	if err := shell.Start(); err != nil {
		return err
	}
	// Wait for process to exit
	return shell.Wait()
}
