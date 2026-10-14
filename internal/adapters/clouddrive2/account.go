package clouddrive2

import (
	"context"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
	"time"
)

type Space struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Free  int64 `json:"free"`
}
type Membership struct {
	Identity  string     `json:"identity"`
	Level     string     `json:"level"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
type AccountInfo struct {
	Path            string       `json:"path"`
	Space           *Space       `json:"space"`
	Memberships     []Membership `json:"memberships"`
	SpaceError      string       `json:"spaceError,omitempty"`
	MembershipError string       `json:"membershipError,omitempty"`
	CheckedAt       time.Time    `json:"checkedAt"`
	Source          string       `json:"source"`
}

// Read-only official RPCs scoped to the connected provider path. Absence and
// unsupported responses stay unknown; zero quota or lifetime are never invented.
func (a *Adapter) AccountInfo(ctx context.Context, p string) (AccountInfo, error) {
	out := AccountInfo{Path: p, Memberships: []Membership{}, Source: "CloudDrive2 官方空间 / 会员接口", CheckedAt: time.Now()}
	ctx, release, err := a.acquire(ctx)
	if err != nil {
		return out, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, a.options.RequestTimeout)
	defer cancel()
	space, err := authorized(ctx, a, func(c context.Context) (*pb.SpaceInfo, error) {
		return a.client.GetSpaceInfo(c, &pb.FileRequest{Path: p})
	})
	if err != nil {
		out.SpaceError = sanitizeError(err).Error()
	} else if space == nil || space.TotalSpace < 0 || space.UsedSpace < 0 || space.FreeSpace < 0 || space.UsedSpace > space.TotalSpace {
		out.SpaceError = storage.ErrInvalidResponse.Error()
	} else {
		out.Space = &Space{space.TotalSpace, space.UsedSpace, space.FreeSpace}
	}
	member, err := authorized(ctx, a, func(c context.Context) (*pb.CloudMemberships, error) {
		return a.client.GetCloudMemberships(c, &pb.FileRequest{Path: p})
	})
	if err != nil {
		out.MembershipError = sanitizeError(err).Error()
	} else if member == nil || len(member.Memberships) > 20 {
		out.MembershipError = storage.ErrInvalidResponse.Error()
	} else {
		for _, m := range member.Memberships {
			if m == nil {
				continue
			}
			item := Membership{Identity: m.Identity, Level: m.GetLevel()}
			if m.ExpireTime != nil && m.ExpireTime.IsValid() {
				t := m.ExpireTime.AsTime()
				item.ExpiresAt = &t
			}
			out.Memberships = append(out.Memberships, item)
		}
	}
	return out, nil
}
