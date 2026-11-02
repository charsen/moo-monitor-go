//go:build !darwin && !linux

package crashoutput

import "os"

func supported() bool                               { return false }
func openReport(*os.Root, string) (*os.File, error) { return nil, ErrRead }
