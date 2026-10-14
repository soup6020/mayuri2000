package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const testTarget = "__sandbox-test-target"

func TestMain(m *testing.M) {
	Main()
	if len(os.Args) > 2 && os.Args[1] == testTarget {
		fmt.Print(fakeDecoder(os.Args[2], os.Args[3:]))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeDecoder stands in for ImageMagick/ffmpeg: it attempts one action from
// inside the sandbox and reports "ok" or the errno it got.
func fakeDecoder(action string, args []string) string {
	result := func(err error) string {
		var errno syscall.Errno
		switch {
		case err == nil:
			return "ok"
		case errors.As(err, &errno):
			return errno.Error()
		}
		return err.Error()
	}
	switch action {
	case "read":
		_, err := os.ReadFile(args[0])
		return result(err)
	case "write":
		return result(os.WriteFile(args[0], []byte("x"), 0o600))
	case "socket":
		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		if err == nil {
			_ = syscall.Close(fd)
		}
		return result(err)
	case "unshare":
		return result(syscall.Unshare(syscall.CLONE_NEWUSER))
	case "ptrace":
		return result(syscall.PtraceAttach(os.Getppid()))
	case "env":
		return strings.Join(os.Environ(), "\n")
	}
	return "unknown action"
}

func requireLandlock(t *testing.T) {
	t.Helper()
	if LandlockABI() < 1 {
		t.Skip("Landlock is not available in this kernel")
	}
}

// run executes the fake decoder in a sandbox with job directory dir.
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary lives outside /nix/store; let the launcher exec it.
	AllowReadOnly(filepath.Dir(self))
	t.Cleanup(reset)
	cmd, err := Command(t.Context(), dir, self, append([]string{testTarget}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("sandboxed run %v: %v: %s", args, err, stderr.String())
	}
	return stdout.String()
}

func TestSandboxConfinesDecoder(t *testing.T) {
	requireLandlock(t)
	job, other := t.TempDir(), t.TempDir()
	self, err := os.Executable() // its directory is read-only allowed (see run)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(other, "token")
	if err := os.WriteFile(secret, []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"write in job dir", []string{"write", filepath.Join(job, "out.png")}, "ok"},
		{"read allowed read-only dir", []string{"read", self}, "ok"},
		{"read /proc", []string{"read", "/proc/meminfo"}, "permission denied"},
		{"read secret outside job dir", []string{"read", secret}, "permission denied"},
		{"write outside job dir", []string{"write", filepath.Join(other, "x")}, "permission denied"},
		{"read /etc", []string{"read", "/etc/passwd"}, "permission denied"},
		{"read parent environment", []string{"read", fmt.Sprintf("/proc/%d/environ", os.Getpid())}, "permission denied"},
		{"network socket", []string{"socket"}, "operation not permitted"},
		{"user namespace", []string{"unshare"}, "operation not permitted"},
		{"ptrace the launcher's parent", []string{"ptrace"}, "operation not permitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, job, tc.args...); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSandboxJobsIsolated(t *testing.T) {
	requireLandlock(t)
	a, b := t.TempDir(), t.TempDir()
	if got := run(t, a, "write", filepath.Join(a, "frame.png")); got != "ok" {
		t.Fatalf("job a writing its own dir: %s", got)
	}
	if got := run(t, b, "read", filepath.Join(a, "frame.png")); got != "permission denied" {
		t.Fatalf("job b reading job a's file: %s", got)
	}
}

func TestSandboxEnvironment(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "secret-token")
	t.Setenv("GENIUS_TOKEN", "secret-genius")
	t.Setenv("TENOR_API_KEY", "secret-tenor")
	dir := t.TempDir()
	SetMode(BestEffort)
	SetEnv("MAGICK_CONFIGURE_PATH", "/old")
	SetEnv("MAGICK_CONFIGURE_PATH", "/magick-config")
	t.Cleanup(reset)
	want := []string{"HOME=" + dir, "TMPDIR=" + dir, "MAGICK_TEMPORARY_PATH=" + dir, "LC_ALL=C", "MAGICK_CONFIGURE_PATH=/magick-config"}
	if got := Env(dir); !equal(got, want) {
		t.Fatalf("Env = %v, want %v", got, want)
	}

	requireLandlock(t)
	got := run(t, dir, "env")
	for _, secret := range []string{"secret-token", "secret-genius", "secret-tenor", "DISCORD_TOKEN"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sandboxed environment leaks %s:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "TMPDIR="+dir) {
		t.Fatalf("sandboxed environment missing TMPDIR:\n%s", got)
	}
}

func TestProbe(t *testing.T) {
	requireLandlock(t)
	r, err := Probe(t.Context())
	if err != nil {
		t.Fatalf("probe: %v (%+v)", err, r)
	}
	if !r.Complete() || r.LandlockABI < 1 {
		t.Fatalf("incomplete sandbox: %+v", r)
	}
	t.Logf("sandbox: %+v", r)
}

func TestPolicyRoundTrip(t *testing.T) {
	SetMode(BestEffort)
	AllowReadOnly("/extra")
	AllowReadOnly("/cfg")
	AllowReadOnly("/extra")
	t.Cleanup(reset)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := Command(t.Context(), "/job", self, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[1] != marker || cmd.Args[3] != "--" || cmd.Args[4] != self || cmd.Dir != "/job" {
		t.Fatalf("args %v dir %s", cmd.Args, cmd.Dir)
	}
	var p policy
	if err := json.Unmarshal([]byte(cmd.Args[2]), &p); err != nil {
		t.Fatal(err)
	}
	if p.Mode != BestEffort || p.Dir != "/job" || !equal(p.RO, []string{"/extra", "/cfg"}) {
		t.Fatalf("policy %+v", p)
	}
	if _, err := ParseMode("off"); err == nil {
		t.Fatal("ParseMode accepted off")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
