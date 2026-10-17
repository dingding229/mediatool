package nativelibrary

import (
	"context"
	"errors"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"time"
)

// PersonalService carries the request-scoped T42 checker, unlike the legacy
// T06 skeleton. No caller may replace it with a client-supplied library list.
type PersonalService interface {
	PersonalData(context.Context, contracts.User, contracts.PermissionChecker, string, *PersonalPatch) (dto.UserItemDataDto, error)
}
type PersonalPatch struct {
	PositionTicks    *int64
	PlayCount        *int
	Played, Favorite *bool
	LastPlayedAt     *time.Time
}

var ErrPersonalBatchTooLarge = errors.New("native user data batch exceeds 5000 episodes")

// CompleteUserData is pure. Key is the stable numeric item ID, never a cloud
// identity. Unknown dates remain omitted per T05; unknown duration has 0 percent.
func CompleteUserData(d dto.UserItemDataDto, id string, runtime *int64) dto.UserItemDataDto {
	d.Key = id
	if d.PlaybackPositionTicks == nil {
		d.PlaybackPositionTicks = ptr(int64(0))
	}
	if d.PlayCount == nil {
		d.PlayCount = ptr(0)
	}
	if d.PlayedPercentage == nil {
		value := float64(0)
		if d.Played {
			value = 100
		} else if runtime != nil && *runtime > 0 {
			value = min(100, max(0, 100*float64(*d.PlaybackPositionTicks)/float64(*runtime)))
		}
		d.PlayedPercentage = &value
	}
	return d
}

func completeProjectionUserData(item *dto.BaseItemDto) {
	data := dto.UserItemDataDto{}
	if item.UserData != nil {
		data = *item.UserData
	}
	data = CompleteUserData(data, item.Id, item.RunTimeTicks)
	item.UserData = &data
}
