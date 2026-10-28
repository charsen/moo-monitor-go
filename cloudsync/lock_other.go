//go:build !darwin && !linux

package cloudsync

func syncLockFlags() int { return 0 } // Buffer construction already rejects unverified platforms.
