package httpapi

import (
	"bytes"
	"encoding/json"
	"media-tool/internal/modules/mediausers"
	"os"
	"testing"
)

func TestMediaUserPasswordPolicyContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/contracts/media-user-password-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Create mediausers.Input `json:"create"`
		Reset  struct {
			Revision           int64  `json:"revision"`
			Password           string `json:"password"`
			MustChangePassword bool   `json:"mustChangePassword"`
		} `json:"reset"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Create.Validate(true); err != nil {
		t.Fatal(err)
	}
	if fixture.Create.MustChangePassword == nil || *fixture.Create.MustChangePassword || !fixture.Reset.MustChangePassword {
		t.Fatal("password policy contract drift")
	}
}

type permissionContract struct {
	OmittedUpdate mediausers.Input `json:"omittedUpdate"`
	ClearUpdate   mediausers.Input `json:"clearUpdate"`
	Update        mediausers.Input `json:"update"`
	Response      mediausers.User  `json:"response"`
}

func readPermissionContract(t *testing.T) permissionContract {
	t.Helper()
	raw, err := os.ReadFile("testdata/contracts/media-user-permissions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture permissionContract
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Update.Validate(false); err != nil {
		t.Fatal(err)
	}
	return fixture
}
func TestMediaUserPermissionContract(t *testing.T) {
	f := readPermissionContract(t)
	if f.Update.MaxStreams == nil || *f.Update.MaxStreams != f.Response.MaxStreams || len(f.Update.Grants) != 1 || !f.Update.Grants[0].CanDownload {
		t.Fatal("permission contract drift")
	}
}
