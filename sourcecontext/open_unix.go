//go:build darwin || linux

package sourcecontext

import (
	"os"
	"syscall"
)

func supported() bool { return true }
func openFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
