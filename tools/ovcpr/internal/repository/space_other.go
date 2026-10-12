//go:build !windows

package repository

import (
	"fmt"
	"syscall"
)

func ensureSpace(path string, needed int64) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return err
	}
	if uint64(st.Bavail)*uint64(st.Bsize) < uint64(needed) {
		return fail("RESOURCE_LIMIT_EXCEEDED", "", fmt.Errorf("insufficient disk space"))
	}
	return nil
}
