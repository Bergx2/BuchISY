//go:build !windows

package core

// limitWorkerMemory is a no-op on non-Windows platforms: a hard committed-memory
// cap for a Go process isn't reliably available (RLIMIT_AS caps reserved virtual
// address space, which Go over-reserves, so it would break the runtime rather
// than bound live memory). The parent's kill-on-timeout is the memory backstop
// there.
func limitWorkerMemory(maxBytes uintptr) {}
