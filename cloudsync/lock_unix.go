//go:build darwin || linux

package cloudsync

import (
	"golang.org/x/sys/unix"
	"os"
)

func syncLockFlags() int { return os.O_RDWR | unix.O_NOFOLLOW }
