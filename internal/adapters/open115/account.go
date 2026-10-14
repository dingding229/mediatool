package open115

import (
	"context"
	"media-tool/internal/modules/storage"
	"strconv"
	"time"
)

type AccountSpace struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Free  int64 `json:"free"`
}
type AccountMembership struct {
	Identity  string     `json:"identity"`
	Level     string     `json:"level"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
type AccountInfo struct {
	UserID          string              `json:"userId"`
	UserName        string              `json:"userName"`
	Path            string              `json:"path"`
	Space           *AccountSpace       `json:"space"`
	Memberships     []AccountMembership `json:"memberships"`
	SpaceError      string              `json:"spaceError,omitempty"`
	MembershipError string              `json:"membershipError,omitempty"`
	Source          string              `json:"source"`
	CheckedAt       time.Time           `json:"checkedAt"`
}
type accountSize struct {
	Size ID `json:"size"`
}

func (c Client) AccountInfo(ctx context.Context, token string) (AccountInfo, error) {
	out := AccountInfo{Source: "115 官方开放平台 /open/user/info", CheckedAt: time.Now(), Memberships: []AccountMembership{}}
	var data struct {
		UserID   ID     `json:"user_id"`
		UserName string `json:"user_name"`
		Space    *struct {
			Total  *accountSize `json:"all_total"`
			Remain *accountSize `json:"all_remain"`
			Use    *accountSize `json:"all_use"`
		} `json:"rt_space_info"`
		VIP *struct {
			LevelName string `json:"level_name"`
			Expire    ID     `json:"expire"`
		} `json:"vip_info"`
	}
	if err := c.call(ctx, "GET", APIOrigin+"/open/user/info", token, "", nil, "", &data); err != nil {
		return out, err
	}
	uid, err := strconv.ParseUint(string(data.UserID), 10, 64)
	if err != nil || uid == 0 {
		return out, storage.ErrInvalidResponse
	}
	out.UserID = string(data.UserID)
	out.UserName = data.UserName
	out.Path = "115账号:" + out.UserID
	parse := func(v *accountSize) (int64, bool) {
		if v == nil {
			return 0, false
		}
		n, err := strconv.ParseInt(string(v.Size), 10, 64)
		return n, err == nil && n >= 0
	}
	if data.Space != nil {
		t, tok := parse(data.Space.Total)
		f, fok := parse(data.Space.Remain)
		u, uok := parse(data.Space.Use)
		if tok && fok && uok && u <= t && f <= t {
			out.Space = &AccountSpace{Total: t, Used: u, Free: f}
		}
	}
	if out.Space == nil {
		out.SpaceError = "115接口未返回完整有效空间数据；不通过CloudDrive2兜底"
	}
	if data.VIP != nil && data.VIP.LevelName != "" {
		m := AccountMembership{Identity: "115", Level: data.VIP.LevelName}
		expires, err := strconv.ParseInt(string(data.VIP.Expire), 10, 64)
		if err == nil && expires > 0 && expires < 253402300800 {
			t := time.Unix(expires, 0)
			m.ExpiresAt = &t
		}
		out.Memberships = append(out.Memberships, m)
	} else {
		out.MembershipError = "115接口未返回会员信息"
	}
	return out, nil
}
