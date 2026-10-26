package onlineupdate

import (
	"os"
	"strings"
	"testing"
)

func TestAvailabilityReasonsWithoutWeakeningAdmission(t *testing.T) {
	for _, c := range []struct {
		mounted bool
		err     error
		want    string
	}{{false, os.ErrNotExist, "未挂载"}, {true, os.ErrPermission, "GID"}, {true, ErrUnsafe, "部署身份"}} {
		if text := availabilityReason(c.mounted, c.err); !strings.Contains(text, c.want) || !strings.Contains(text, "检查更新") {
			t.Fatal(text)
		}
	}
}
