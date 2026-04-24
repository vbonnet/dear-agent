//go:build linux
// +build linux

package security

// SeccompFilter represents syscall filtering rules
type SeccompFilter struct {
	allowedSyscalls []string
}

// NewSeccompFilter creates a new seccomp filter with safe defaults
func NewSeccompFilter(permissions Permissions) *SeccompFilter {
	filter := &SeccompFilter{
		allowedSyscalls: getBaseSyscalls(),
	}

	// Add network syscalls if network is allowed
	if len(permissions.Network) > 0 {
		filter.allowedSyscalls = append(filter.allowedSyscalls, getNetworkSyscalls()...)
	}

	return filter
}

// getBaseSyscalls returns the base set of allowed syscalls
// These are essential for basic program execution
func getBaseSyscalls() []string {
	return []string{
		// Process control
		"exit",
		"exit_group",
		"wait4",
		"waitid",

		// Memory management
		"brk",
		"mmap",
		"munmap",
		"mprotect",
		"mremap",
		"madvise",

		// File I/O
		"read",
		"write",
		"open",
		"openat",
		"close",
		"stat",
		"fstat",
		"lstat",
		"lseek",
		"access",
		"faccessat",
		"readlink",
		"readlinkat",
		"getcwd",
		"chdir",
		"fchdir",

		// Directory operations
		"getdents",
		"getdents64",
		"mkdir",
		"mkdirat",
		"rmdir",

		// File metadata
		"chmod",
		"fchmod",
		"fchmodat",
		"chown",
		"fchown",
		"fchownat",
		"utime",
		"utimes",
		"futimesat",

		// Pipes and IPC
		"pipe",
		"pipe2",
		"dup",
		"dup2",
		"dup3",

		// Signal handling
		"rt_sigaction",
		"rt_sigprocmask",
		"rt_sigreturn",
		"sigaltstack",

		// Process info
		"getpid",
		"gettid",
		"getppid",
		"getuid",
		"geteuid",
		"getgid",
		"getegid",
		"getgroups",

		// Time
		"gettimeofday",
		"clock_gettime",
		"clock_getres",
		"nanosleep",

		// Threading (if needed)
		"clone",
		"set_tid_address",
		"set_robust_list",
		"futex",

		// Misc
		"uname",
		"arch_prctl",
		"prctl",
		"getrandom",
	}
}

// getNetworkSyscalls returns syscalls needed for network operations
func getNetworkSyscalls() []string {
	return []string{
		"socket",
		"connect",
		"bind",
		"listen",
		"accept",
		"accept4",
		"sendto",
		"recvfrom",
		"sendmsg",
		"recvmsg",
		"setsockopt",
		"getsockopt",
		"shutdown",
		"getpeername",
		"getsockname",
	}
}

// ValidateSeccompSupport checks if seccomp is available
func ValidateSeccompSupport() error {
	// Check if seccomp is supported by the kernel
	// This requires checking /proc/self/status for Seccomp field
	// For now, assume it's available on Linux

	return nil
}

// Note on Seccomp Implementation:
//
// Full seccomp-bpf implementation requires:
// 1. Dependency: github.com/seccomp/libseccomp-golang
// 2. Build BPF program:
//    filter := seccomp.NewFilter(seccomp.ActErrno)
//    for _, syscall := range allowedSyscalls {
//        filter.AddRule(seccomp.ActAllow, syscallName)
//    }
//    filter.Load()
//
// 3. Apply before exec:
//    cmd.SysProcAttr.AmbientCaps = []uintptr{}
//    cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWNS
//
// This is deferred to Phase 1.2 iteration 2 to avoid dependency bloat.
// Current implementation focuses on AppArmor which is more widely deployed.
