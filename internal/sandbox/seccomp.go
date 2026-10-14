package sandbox

import (
	"fmt"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/sys/unix"
)

// denied are syscalls a decoder never needs: networking, access to other
// processes, namespaces and mounts, and kernel interfaces that are common
// privilege-escalation surface. Everything else is allowed: an allowlist
// would break on codec- and format-specific syscall paths. Names unknown on
// the running architecture are skipped.
var denied = []string{
	// Networking (decoders only read and write files and pipes).
	"socket", "socketpair",
	// Other processes.
	"ptrace", "process_vm_readv", "process_vm_writev", "pidfd_getfd", "kcmp",
	// Namespaces, mounts and filesystem handles.
	"unshare", "setns", "mount", "umount2", "pivot_root", "chroot",
	"fsopen", "fsconfig", "fsmount", "fspick", "move_mount", "open_tree", "mount_setattr",
	"open_by_handle_at", "name_to_handle_at",
	// Kernel interfaces.
	"init_module", "finit_module", "delete_module", "kexec_load", "kexec_file_load",
	"bpf", "perf_event_open", "userfaultfd",
	"io_uring_setup", "io_uring_enter", "io_uring_register",
	"keyctl", "add_key", "request_key",
	"iopl", "ioperm", "swapon", "swapoff", "reboot", "acct", "quotactl",
	"settimeofday", "clock_settime", "clock_adjtime", "adjtimex",
	"lookup_dcookie", "vhangup",
}

// Namespace flags that make clone(2) create namespaces.
var cloneNamespaceFlags = []uint64{
	unix.CLONE_NEWNS, unix.CLONE_NEWCGROUP, unix.CLONE_NEWUTS, unix.CLONE_NEWIPC,
	unix.CLONE_NEWUSER, unix.CLONE_NEWPID, unix.CLONE_NEWNET,
}

func seccompPolicy() (seccomp.Policy, error) {
	info, err := arch.GetInfo("")
	if err != nil {
		return seccomp.Policy{}, err
	}
	var names []string
	for _, n := range denied {
		if _, ok := info.SyscallNames[n]; ok {
			names = append(names, n)
		}
	}
	var cloneConds []seccomp.NameWithConditions
	for _, f := range cloneNamespaceFlags {
		cloneConds = append(cloneConds, seccomp.NameWithConditions{
			Name:       "clone",
			Conditions: seccomp.ArgumentConditions{{Argument: 0, Operation: seccomp.BitsSet, Value: f}},
		})
	}
	groups := []seccomp.SyscallGroup{
		{Names: names, Action: seccomp.ActionErrno},
		{NamesWithCondtions: cloneConds, Action: seccomp.ActionErrno},
	}
	// clone3 passes flags in memory, where BPF can't inspect them. Answer
	// ENOSYS so libc falls back to clone(2), which is checked above.
	if _, ok := info.SyscallNames["clone3"]; ok {
		groups = append(groups, seccomp.SyscallGroup{
			Names: []string{"clone3"}, Action: seccomp.ActionErrno | seccomp.Action(unix.ENOSYS),
		})
	}
	return seccomp.Policy{DefaultAction: seccomp.ActionAllow, Syscalls: groups}, nil
}

func loadSeccomp() error {
	if !seccomp.Supported() {
		return fmt.Errorf("seccomp is not supported by this kernel")
	}
	p, err := seccompPolicy()
	if err != nil {
		return err
	}
	return seccomp.LoadFilter(seccomp.Filter{NoNewPrivs: true, Flag: seccomp.FilterFlagTSync, Policy: p})
}
