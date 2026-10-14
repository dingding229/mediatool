//go:build !linux && !darwin

package filemanager

import "os"

func directoryWritable(_ *os.File) (bool, error) { return false, nil }
