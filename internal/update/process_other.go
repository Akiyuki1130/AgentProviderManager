//go:build !windows

package update

import "errors"

func startProcess(string, []string) error {
	return errors.New("starting the update helper is only supported on Windows")
}
