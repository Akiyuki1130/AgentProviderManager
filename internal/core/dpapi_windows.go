//go:build windows

package core

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func dpapiWindows(data []byte, protect bool) ([]byte, error) {
	input := windows.DataBlob{Size: uint32(len(data))}
	if len(data) > 0 {
		input.Data = &data[0]
	}
	output := windows.DataBlob{}
	var err error
	if protect {
		err = windows.CryptProtectData(&input, nil, nil, 0, nil, 0, &output)
	} else {
		err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, 0, &output)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if output.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
		}
	}()
	if output.Data == nil || output.Size == 0 {
		return []byte{}, nil
	}
	result := make([]byte, output.Size)
	copy(result, unsafe.Slice(output.Data, output.Size))
	return result, nil
}
