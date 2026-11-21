package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Production fails closed when namespace isolation is unavailable. "none" is
// an explicit local-development opt-out, never set by the supplied Compose.
func (rn *runner) sandboxCommand(dir, name string, args []string) (string, []string, error) {
	if rn.sandboxMode == "none" {
		return name, args, nil
	}
	if rn.sandboxMode != "bubblewrap" || runtime.GOOS != "linux" {
		return "", nil, fmt.Errorf("изолированное выполнение требует Linux и bubblewrap")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return "", nil, fmt.Errorf("bubblewrap не установлен: выполнение кода отключено")
	}
	interpreter, err := exec.LookPath(name)
	if err != nil {
		return "", nil, err
	}
	interpreter, err = filepath.EvalSymlinks(interpreter)
	if err != nil {
		return "", nil, err
	}
	isolated := []string{
		"--unshare-all", "--die-with-parent", "--new-session", "--cap-drop", "ALL",
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib64", "/lib64",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
		"--dir", "/opt", "--ro-bind", interpreter, "/opt/interpreter",
		"--bind", dir, "/work", "--chdir", "/work",
		"--clearenv", "--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/work", "--setenv", "TMPDIR", "/tmp",
		"--setenv", "LANG", "C.UTF-8", "--setenv", "PYTHONIOENCODING", "utf-8",
		"--", "/opt/interpreter",
	}
	return bwrap, append(isolated, args...), nil
}
