// SPDX-License-Identifier: GPL-3.0-or-later

package repository

import (
	"fmt"
	"syscall"
	"unsafe"
)

func ensureSpace(path string, needed int64) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var available uint64
	call := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	ok, _, err := call.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), 0, 0)
	if ok == 0 {
		return err
	}
	if available < uint64(needed) {
		return fail("RESOURCE_LIMIT_EXCEEDED", "", fmt.Errorf("Available storage is insufficient."))
	}
	return nil
}
