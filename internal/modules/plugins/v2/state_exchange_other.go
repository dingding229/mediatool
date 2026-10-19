//go:build !darwin && !linux

package v2

func exchangeDirs(int, string, string) error { return ErrUnavailable }
