package nativeauth

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"strings"
	"testing"
)

func TestAuthenticationInputsAndUnavailableStoreFailClosed(t *testing.T) {
	valid := contracts.DeviceInfo{DeviceID: "synthetic-device", Name: "Synthetic iPad", Client: "Infuse", Version: "8.0", ClientIP: "93.184.216.34", UserAgent: "SyntheticPlayer/1"}
	if !validInfo(valid, true) || validInfo(contracts.DeviceInfo{}, true) || !validInfo(contracts.DeviceInfo{}, false) {
		t.Fatal("device rules changed")
	}
	for _, mutate := range []func(*contracts.DeviceInfo){func(d *contracts.DeviceInfo) { d.DeviceID = strings.Repeat("a", 257) }, func(d *contracts.DeviceInfo) { d.Client = strings.Repeat("a", 129) }, func(d *contracts.DeviceInfo) { d.Version = strings.Repeat("1", 65) }, func(d *contracts.DeviceInfo) { d.UserAgent = strings.Repeat("a", 2049) }, func(d *contracts.DeviceInfo) { d.Name = "bad\nname" }} {
		info := valid
		mutate(&info)
		if validInfo(info, true) {
			t.Fatal("unsafe metadata accepted")
		}
	}
	var store *Store
	if _, err := store.Login(context.Background(), "synthetic.member", "password123", valid); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal("nil database login was accepted")
	}
	if _, _, _, err := store.Authenticate(context.Background(), "invalid", valid); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal("nil database authentication was accepted")
	}
	if err := store.Logout(context.Background(), contracts.Token{}); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := store.ChangePassword(context.Background(), contracts.User{}, contracts.Token{}, "password123", "nextpass123"); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := store.PublicUsers(context.Background()); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := store.GetUser(context.Background(), "synthetic"); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	if ipValue("unknown") != nil || ipValue("::ffff:93.184.216.34") != "93.184.216.34" {
		t.Fatal("IP evidence handling changed")
	}
}
func TestRawTokenOnlyHasExplicitProtocolSerialization(t *testing.T) {
	raw, err := json.Marshal(contracts.LoginResult{AccessToken: "synthetic-secret-which-must-not-be-marshaled"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic-secret") || strings.Contains(string(raw), "AccessToken") {
		t.Fatal("internal login envelope leaked plaintext token")
	}
}
