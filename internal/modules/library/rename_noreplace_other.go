//go:build !linux && !darwin

package library

import (
	"errors"
	"os"
)

func renameNoReplace(from *os.File, fromName string, to *os.File, toName string) error {
	return errors.New("output archival requires atomic no-replace rename support")
}
