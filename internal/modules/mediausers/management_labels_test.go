package mediausers

import (
	"strings"
	"testing"
)

func TestManagementLabelsDoNotReflectClientSecrets(t *testing.T) {
	for _, s := range []string{"Synthetic iPad", "Infuse/8.0", "Synthetic Show s1e2", "8.0", "来自媒体用户的设备"} {
		if ManagementLabel(s) != s {
			t.Fatal("safe label hidden", s)
		}
	}
	for _, s := range []string{"/115/private.mkv", "设备 https://resolver.invalid/strm302?apiKey=private", "pickCode=private", "api_key=private", "/root/private", "%2F115%2Fprivate", "C:\\private", strings.Repeat("a", 64), "Device_" + strings.Repeat("b", 64), strings.Repeat("中", 257), "bad\nname"} {
		if ManagementLabel(s) != "[名称已隐藏]" {
			t.Fatal("sensitive label retained", s)
		}
	}
}
