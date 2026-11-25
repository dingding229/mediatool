package accounts

import (
	"strings"
	"testing"
)

func TestRP31InitializationTokenSyntax(t *testing.T) {
	valid := strings.Repeat("A", 43)
	for _, token := range []string{"", valid[:42], valid + "A", valid[:42] + "B", "\n" + valid[:42], valid[:42] + "="} {
		if ValidInitializationToken(token) {
			t.Fatal("accepted noncanonical token format")
		}
	}
	if !ValidInitializationToken(valid) {
		t.Fatal("rejected canonical token format")
	}
}
