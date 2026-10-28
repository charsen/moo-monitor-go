//go:build darwin || linux

package buffer

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockFlags() int { return os.O_RDWR | unix.O_NOFOLLOW }
