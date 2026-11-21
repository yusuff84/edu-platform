package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDescendantDoesNotHoldRequestOpen(t *testing.T) {
	requireBin(t, "python3")
	rn := newTestRunner()
	rn.execTimeout = 250 * time.Millisecond
	dir := t.TempDir()
	start := time.Now()
	_, _ = rn.runCmd(context.Background(), dir, "", "python3", "-c", `import subprocess,time
p=subprocess.Popen(['python3','-c','import time;time.sleep(30)'])
open('child.pid','w').write(str(p.pid))
time.sleep(30)`)
	if time.Since(start) > 2*time.Second {
		t.Fatal("descendant held output pipe past deadline")
	}
	data, err := os.ReadFile(dir + "/child.pid")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		// A killed child may remain briefly as a zombie until init reaps it.
		stat, readErr := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if readErr == nil && !strings.Contains(string(stat), ") Z ") {
			t.Fatalf("child %d survived", pid)
		}
	}
}
