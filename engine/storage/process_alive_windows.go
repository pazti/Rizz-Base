//go:build windows

package storage

import (
	"fmt"
	"syscall"
	"unsafe"
)

const processQueryLimitedInformation = 0x1000
const processStillActive = 259
const errorInvalidParameter = syscall.Errno(87)

func processAlive(pid int) (bool, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	getExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
	closeHandle := kernel32.NewProc("CloseHandle")

	handle, _, openErr := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		if openErr == errorInvalidParameter {
			return false, nil
		}
		return false, fmt.Errorf("OpenProcess: %w", openErr)
	}
	defer closeHandle.Call(handle)

	var exitCode uint32
	ok, _, exitErr := getExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	if ok == 0 {
		return false, fmt.Errorf("GetExitCodeProcess: %w", exitErr)
	}
	return exitCode == processStillActive, nil
}
