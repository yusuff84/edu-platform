package main

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxFailsClosed(t *testing.T) {
	rn := newRunner()
	rn.sandboxMode = "unsupported"
	if _, _, err := rn.sandboxCommand(t.TempDir(), "python3", nil); err == nil {
		t.Fatal("unsupported sandbox must not execute code")
	}
}

func TestSandboxIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux namespaces required")
	}
	rn := newRunner()
	rn.sandboxMode = "bubblewrap"
	dir := t.TempDir()
	probe, err := rn.runCmd(context.Background(), dir, "", "python3", "-c", "print('ready')")
	if err != nil || strings.TrimSpace(probe) != "ready" {
		if os.Getenv("RUNNER_REQUIRE_SANDBOX_TEST") == "1" {
			t.Fatalf("sandbox unavailable: %v: %s", err, probe)
		}
		t.Skipf("host does not support sandbox: %v: %s", err, probe)
	}
	// Exercise the real HTTP file execution path as well as the -c probe.
	// Host absolute solution paths are unavailable after binding /work.
	for _, body := range []string{
		`{"language":"python","code":"print(42)"}`,
		`{"language":"python","code":"print(42)","tests":[{"name":"file","input":"","expected":"42"}]}`,
		`{"language":"js","code":"console.log(42)"}`,
		`{"language":"js","code":"console.log(42)","tests":[{"name":"file","input":"","expected":"42"}]}`,
	} {
		rec := doRun(t, rn, body)
		var result runResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || !result.Passed {
			t.Fatalf("sandbox file execution failed: %d %s", rec.Code, rec.Body.String())
		}
	}
	// The host-only marker is not mounted, network has only loopback, and
	// /proc exposes only the check's own PID namespace.
	marker, err := os.CreateTemp("", "runner-host-secret-")
	if err != nil {
		t.Fatal(err)
	}
	marker.Close()
	defer os.Remove(marker.Name())
	code := `import os,socket
assert not os.path.exists(` + "'" + marker.Name() + "'" + `)
assert len([x for x in os.listdir('/proc') if x.isdigit()]) < 10
assert os.listdir('/sys/class/net') == [] if os.path.exists('/sys/class/net') else True
s=socket.socket();s.settimeout(0.1)
assert s.connect_ex(('192.0.2.1',8080)) != 0
print('isolated')`
	out, err := rn.runCmd(context.Background(), dir, "", "python3", "-c", code)
	if err != nil || strings.TrimSpace(out) != "isolated" {
		t.Fatalf("isolation failed: %v: %s", err, out)
	}
}
