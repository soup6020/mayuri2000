// Package sandbox runs untrusted-input decoders (ImageMagick, ffmpeg) with
// only what they need: a fixed minimal environment, filesystem access limited
// by Landlock to the program files and the job's own directory, a seccomp
// filter denying networking and kernel attack surface, and resource limits.
//
// It works without privileges (no namespaces), so it also works inside a
// hardened container. Commands re-exec the bot binary as a small launcher
// (see Main) that restricts itself and then execs the decoder; all of the
// restrictions carry across execve.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
)

// Mode decides what happens when part of the sandbox can't be applied.
type Mode string

const (
	// Required refuses to run a decoder unless Landlock and seccomp both apply.
	Required Mode = "required"
	// BestEffort applies whatever the kernel supports.
	BestEffort Mode = "best-effort"
)

// ParseMode validates a DECODER_SANDBOX value; empty means Required.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", Required:
		return Required, nil
	case BestEffort:
		return BestEffort, nil
	}
	return "", fmt.Errorf("DECODER_SANDBOX must be %s or %s, not %q", Required, BestEffort, s)
}

const marker = "__mayuri-sandbox"

// policy is what the launcher applies; it travels as one argv element.
type policy struct {
	Mode Mode     `json:"mode"`
	Dir  string   `json:"dir"`          // the job directory: the only writable path
	RO   []string `json:"ro,omitempty"` // extra read-only directories
}

var (
	mu       sync.RWMutex
	mode     = Required
	exe      string
	exeErr   error
	exeOnce  sync.Once
	extraRO  []string
	extraEnv []string
)

// SetMode sets what happens when part of the sandbox can't be applied.
func SetMode(m Mode) {
	mu.Lock()
	defer mu.Unlock()
	mode = m
}

// AllowReadOnly lets sandboxed programs read dir (e.g. support files written
// at startup). Only directories holding nothing secret belong here.
func AllowReadOnly(dir string) {
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(extraRO, dir) {
		extraRO = append(extraRO, dir)
	}
}

// SetEnv adds a fixed variable to every sandboxed environment (e.g.
// MAGICK_CONFIGURE_PATH). Never use it for secrets.
func SetEnv(key, value string) {
	mu.Lock()
	defer mu.Unlock()
	extraEnv = slices.DeleteFunc(extraEnv, func(kv string) bool { return strings.HasPrefix(kv, key+"=") })
	extraEnv = append(extraEnv, key+"="+value)
}

// reset restores the defaults (tests only).
func reset() {
	mu.Lock()
	defer mu.Unlock()
	mode, extraRO, extraEnv = Required, nil, nil
}

// CurrentMode reports the configured mode.
func CurrentMode() Mode {
	mu.RLock()
	defer mu.RUnlock()
	return mode
}

func executable() (string, error) {
	exeOnce.Do(func() { exe, exeErr = os.Executable() })
	return exe, exeErr
}

// Env is the complete environment a sandboxed program gets for job dir dir.
// Nothing from the bot's own environment (tokens, API keys) is passed on.
func Env(dir string) []string {
	mu.RLock()
	defer mu.RUnlock()
	env := []string{"HOME=" + dir, "TMPDIR=" + dir, "MAGICK_TEMPORARY_PATH=" + dir, "LC_ALL=C"}
	return append(env, extraEnv...)
}

// Command returns a command that runs program (a path, or a name looked up in
// the bot's PATH) with args inside the sandbox, with dir as its working and
// only writable directory. The caller owns dir and removes it afterwards.
func Command(ctx context.Context, dir, program string, args ...string) (*exec.Cmd, error) {
	self, err := executable()
	if err != nil {
		return nil, fmt.Errorf("locating the sandbox launcher: %w", err)
	}
	// Resolve here: the sandboxed environment has no PATH.
	path, err := exec.LookPath(program)
	if err != nil {
		return nil, err
	}
	mu.RLock()
	p := policy{Mode: mode, Dir: dir, RO: extraRO}
	mu.RUnlock()
	enc, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, self, append([]string{marker, string(enc), "--", path}, args...)...)
	cmd.Env = Env(dir)
	cmd.Dir = dir
	return cmd, nil
}

// Report says which parts of the sandbox took effect.
type Report struct {
	LandlockABI int      `json:"landlock_abi"` // 0 when Landlock is unavailable
	Landlock    bool     `json:"landlock"`
	Seccomp     bool     `json:"seccomp"`
	Rlimits     bool     `json:"rlimits"`
	Problems    []string `json:"problems,omitempty"`
}

// Complete reports whether every part applied.
func (r Report) Complete() bool { return r.Landlock && r.Seccomp && r.Rlimits }

// Probe launches the launcher in check mode and reports what applied. In
// Required mode an incomplete sandbox is returned as an error too.
func Probe(ctx context.Context) (Report, error) {
	dir, err := os.MkdirTemp("", "mayuri-sandbox-probe-*")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(dir)
	self, err := executable()
	if err != nil {
		return Report{}, err
	}
	cmd, err := Command(ctx, dir, self, checkTarget)
	if err != nil {
		return Report{}, err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var r Report
	if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
		return r, errors.Join(fmt.Errorf("sandbox self-test failed: %s", strings.TrimSpace(stderr.String())), runErr)
	}
	if CurrentMode() == Required && !r.Complete() {
		return r, fmt.Errorf("decoder sandbox unavailable: %s", strings.Join(r.Problems, "; "))
	}
	return r, nil
}

// RunTests is TestMain for packages whose tests run decoders: the test
// binary doubles as the sandbox launcher. Without Landlock (e.g. an old CI
// kernel) the tests fall back to best effort instead of failing.
func RunTests(m interface{ Run() int }) {
	Main()
	if LandlockABI() < 1 {
		SetMode(BestEffort)
	}
	os.Exit(m.Run())
}
