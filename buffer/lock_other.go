//go:build !darwin && !linux

package buffer

// Other platforms are deliberately unsupported until their file/directory semantics are verified.
func lockFlags() int { return 0 }
