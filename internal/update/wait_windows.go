//go:build windows

package update

import (
	"context"
	"errors"
	"fmt"
	"syscall"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	openProcess         = kernel32.NewProc("OpenProcess")
	waitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	closeHandle         = kernel32.NewProc("CloseHandle")
)

const (
	processQueryLimitedInformation = 0x1000
	synchronize                    = 0x00100000
	waitObject0                    = 0x00000000
	waitTimeout                    = 0x00000102
	infinite                       = 0xffffffff
)

func waitForParentExit(ctx context.Context, pid int) error {
	h, _, callErr := openProcess.Call(processQueryLimitedInformation|synchronize, 0, uintptr(pid))
	if h == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("open parent process: %w", callErr)
		}
		return errors.New("open parent process failed")
	}
	defer closeHandle.Call(h)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, _, callErr := waitForSingleObject.Call(h, 250)
		if result == 0xffffffff {
			if callErr != syscall.Errno(0) {
				return fmt.Errorf("wait for parent process: %w", callErr)
			}
			return errors.New("wait for parent process failed")
		}
		switch result {
		case waitObject0:
			return nil
		case waitTimeout:
			continue
		default:
			return fmt.Errorf("wait for parent process returned %#x", result)
		}
	}
}
