package repository

import (
	"fmt"
	"os"
	"syscall"
)

func checkHardlink(r *os.Root, p string) error {
	f, err := r.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks > 1 {
		return fmt.Errorf("hardlinks are forbidden")
	}
	return nil
}
