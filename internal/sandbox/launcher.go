package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

// checkTarget makes the launcher report what it applied instead of exec'ing.
const checkTarget = "__mayuri-sandbox-check"

// exitLauncher is the launcher's exit status when it refuses to run.
const exitLauncher = 125

// Main must run first in main() (and in TestMain of packages that run
// decoders). When the process was started as the sandbox launcher it applies
// the sandbox and execs the target, never returning; otherwise it returns.
func Main() {
	if len(os.Args) < 5 || os.Args[1] != marker || os.Args[3] != "--" {
		return
	}
	var p policy
	if err := json.Unmarshal([]byte(os.Args[2]), &p); err != nil || p.Dir == "" {
		fail(fmt.Errorf("invalid sandbox policy: %w", err))
	}
	target, args := os.Args[4], os.Args[4:]

	runtime.LockOSThread()
	r := apply(p)
	if p.Mode != BestEffort && !r.Complete() {
		fail(fmt.Errorf("decoder sandbox unavailable: %v", r.Problems))
	}
	if len(args) > 1 && args[1] == checkTarget {
		check(&r, p)
		_ = json.NewEncoder(os.Stdout).Encode(r)
		os.Exit(0)
	}
	if err := syscall.Exec(target, args, os.Environ()); err != nil {
		fail(fmt.Errorf("exec %s: %w", target, err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sandbox:", err)
	os.Exit(exitLauncher)
}

// apply restricts this process. Order matters: limits first, then Landlock
// (which sets no_new_privs), then the seccomp filter.
func apply(p policy) Report {
	var r Report
	if err := setLimits(); err != nil {
		r.Problems = append(r.Problems, "rlimits: "+err.Error())
	} else {
		r.Rlimits = true
	}

	abi, err := llsys.LandlockGetABIVersion()
	if err != nil || abi < 1 {
		r.Problems = append(r.Problems, "Landlock is not available in this kernel")
	} else {
		r.LandlockABI = abi
		if err := landlock.V10.BestEffort().Restrict(landlockRules(p)...); err != nil {
			r.Problems = append(r.Problems, "Landlock: "+err.Error())
		} else {
			r.Landlock = true
		}
	}

	if err := loadSeccomp(); err != nil {
		r.Problems = append(r.Problems, "seccomp: "+err.Error())
	} else {
		r.Seccomp = true
	}
	return r
}

// Resource limits for one decoder run. CPU matches ImageMagick's own
// "-limit time 150"; a runaway exploit is killed even if it ignores signals.
func setLimits() error {
	limits := []struct {
		resource int
		value    uint64
	}{
		{syscall.RLIMIT_CORE, 0},
		{syscall.RLIMIT_CPU, 150},
		{syscall.RLIMIT_FSIZE, 1 << 30},
		{syscall.RLIMIT_NOFILE, 256},
	}
	var errs []error
	for _, l := range limits {
		lim := syscall.Rlimit{Cur: l.value, Max: l.value}
		if err := syscall.Setrlimit(l.resource, &lim); err != nil {
			errs = append(errs, fmt.Errorf("resource %d: %w", l.resource, err))
		}
	}
	return errors.Join(errs...)
}

// readOnlyDirs are what decoders read besides the job directory, found by
// tracing ImageMagick and ffmpeg (strace -e trace=%file) over the effect and
// video test suites: their programs and libraries in /nix/store, and
// /sys/devices/system/cpu/{possible,online} for thread counts. They also try
// /proc/self/{status,mounts}, /proc/filesystems and /sys/devices/system/node
// (libselinux, NUMA probing) and cope with those being denied. Missing paths
// are skipped.
var readOnlyDirs = []string{"/nix/store", "/sys/devices/system/cpu"}

func landlockRules(p policy) []landlock.Rule {
	ro := append(append([]string(nil), readOnlyDirs...), p.RO...)
	return []landlock.Rule{
		landlock.RODirs(ro...).IgnoreIfMissing(),
		landlock.RWFiles("/dev/null"),
		landlock.RWDirs(p.Dir),
	}
}

// check probes the restrictions from inside, so the report reflects reality
// rather than what the APIs claimed.
func check(r *Report, p policy) {
	if r.Seccomp {
		if fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0); err == nil {
			_ = syscall.Close(fd)
			r.Seccomp = false
			r.Problems = append(r.Problems, "seccomp: socket() was not blocked")
		}
	}
	if r.Landlock {
		// The job directory's parent (the shared temp directory) holds other
		// jobs' directories and must not be listable.
		if _, err := os.ReadDir(filepath.Dir(p.Dir)); err == nil {
			r.Landlock = false
			r.Problems = append(r.Problems, "Landlock: paths outside the job directory were readable")
		}
	}
}

// LandlockABI reports the kernel's Landlock ABI version, 0 if unavailable.
func LandlockABI() int {
	abi, err := llsys.LandlockGetABIVersion()
	if err != nil {
		return 0
	}
	return abi
}
