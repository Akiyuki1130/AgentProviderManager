//go:build !windows

package core

import "fmt"

func dpapiWindows([]byte, bool) ([]byte, error) {
	return nil, fmt.Errorf("Windows DPAPI is unavailable on this platform")
}
