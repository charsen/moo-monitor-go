//go:build !darwin && !linux

package sourcecontext

import "os"

func supported() bool                             { return false }
func openFile(*os.Root, string) (*os.File, error) { return nil, ErrConfig }
