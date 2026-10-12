//go:build !windows

package repository

import (
	"fmt"
	"os"
	"syscall"
)

func checkHardlink(r *os.Root, p string) error {
	st, err := r.Stat(p)
	if err != nil {
		return err
	}
	if v, ok := st.Sys().(*syscall.Stat_t); ok && v.Nlink > 1 {
		return fmt.Errorf("hardlinks are forbidden")
	}
	return nil
}
