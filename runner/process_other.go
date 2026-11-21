//go:build !linux && !windows

package main

import "os/exec"

func configureProcessTree(cmd *exec.Cmd) func() { return func() {} }
