package main

import (
	"os/exec"
	"strconv"
)

func configureProcessTree(cmd *exec.Cmd) func() {
	kill := func() error {
		if cmd.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	cmd.Cancel = kill
	return func() {}
}
