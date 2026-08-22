//go:build !windows

package update

import (
	"context"
	"errors"
)

func waitForParentExit(ctx context.Context, pid int) error {
	_ = ctx
	_ = pid
	return errors.New("update helper installation is only supported on Windows")
}
