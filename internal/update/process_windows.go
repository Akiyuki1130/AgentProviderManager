//go:build windows

package update

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startProcess(path string, args []string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	allArgs := make([]string, 0, len(args)+1)
	allArgs = append(allArgs, path)
	allArgs = append(allArgs, args...)
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(allArgs))
	if err != nil {
		return err
	}
	application, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	workingDir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return err
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(application, commandLine, nil, nil, false, windows.CREATE_NEW_PROCESS_GROUP, nil, workingDir, &startup, &process); err != nil {
		return fmt.Errorf("CreateProcess: %w", err)
	}
	_ = windows.CloseHandle(process.Thread)
	_ = windows.CloseHandle(process.Process)
	return nil
}
