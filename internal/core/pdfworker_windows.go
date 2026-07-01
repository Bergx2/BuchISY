//go:build windows

package core

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// limitWorkerMemory caps the current process's committed memory at maxBytes via
// a Windows Job Object. When a runaway PDF parse pushes commit past the limit,
// the allocation fails and the Go runtime aborts the process — so a pathological
// PDF can never balloon to multiple GB (GOMEMLIMIT can't bound it: the parser
// retains the memory live, defeating the GC). A ProcessMemoryLimit bounds
// *committed* memory, which suits Go (it reserves virtual address space lazily
// commits), so this does not disturb normal startup.
//
// Best-effort: any failure leaves the process unlimited (the parent's timeout is
// still the backstop).
func limitWorkerMemory(maxBytes uintptr) {
	jo, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	// Intentionally keep the job handle open for the process's (short) lifetime
	// so the limit stays enforced; it is released automatically on exit.

	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
	info.ProcessMemoryLimit = maxBytes
	if _, err := windows.SetInformationJobObject(jo, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(jo)
		return
	}
	if err := windows.AssignProcessToJobObject(jo, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(jo)
	}
}
