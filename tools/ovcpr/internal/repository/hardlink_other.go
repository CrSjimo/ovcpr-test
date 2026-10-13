// SPDX-License-Identifier: GPL-3.0-or-later

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
		return fmt.Errorf("Hard links are forbidden.")
	}
	return nil
}
