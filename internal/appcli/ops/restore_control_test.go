package ops

import (
	"strings"
	"testing"
)

func TestObsoleteHostRestoreCannotClaimOrManuallyFinish(t *testing.T) {
	err := requireImageControl("restore")
	if err == nil || !strings.Contains(err.Error(), "共享执行器") {
		t.Fatal(err)
	}
	if err = requireImageControl("rollback"); err != nil {
		t.Fatal("image rollback control changed", err)
	}
}
