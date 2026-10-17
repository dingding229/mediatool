package nativelibrary

import (
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediaprobe/probeinfo"
)

// A source's successful snapshot remains useful while a later attempt is pending
// or failed. No runtime lookup or execution belongs in these pure mappings.
func applyProbeSource(dst *dto.MediaSourceInfo, p *probeinfo.Result) {
	if p == nil || p.Validate() != nil {
		return
	}
	if p.DurationTicks > 0 {
		dst.RunTimeTicks = ptr(p.DurationTicks)
	}
	if p.Container != "" {
		dst.Container = p.Container
	}
	dst.Size, dst.Bitrate = p.Size, p.BitRate
	for _, s := range p.Streams {
		stream := dto.MediaStream{Index: ptr(s.Index), Type: s.Type, Codec: s.Codec, Profile: s.Profile, Level: s.Level, Width: s.Width, Height: s.Height, BitRate: s.BitRate,
			AverageFrameRate: s.AverageFrameRate, RealFrameRate: s.RealFrameRate, PixelFormat: s.PixelFormat, BitDepth: s.BitDepth, AspectRatio: s.AspectRatio,
			VideoRange: s.VideoRange, VideoRangeType: s.VideoRangeType, IsInterlaced: s.IsInterlaced, Rotation: s.Rotation, Channels: s.Channels, ChannelLayout: s.ChannelLayout,
			SampleRate: s.SampleRate, Language: s.Language, Title: s.Title, IsDefault: s.IsDefault, IsForced: s.IsForced, IsHearingImpaired: s.IsHearingImpaired,
			IsTextSubtitleStream: s.IsTextSubtitleStream, DisplayTitle: probeinfo.DisplayTitle(s)}
		if s.Type == probeinfo.Subtitle {
			stream.DeliveryMethod = "Embed"
			stream.IsExternal = false
		}
		dst.MediaStreams = append(dst.MediaStreams, stream)
		if s.Type == probeinfo.Video && dst.Width == nil {
			dst.Width, dst.Height = s.Width, s.Height
		}
	}
}

// Item-level technical data follows the first (existing version-order) source.
// Its absence never borrows a different version's streams or changes fallback.
func applyProbeItem(item *dto.BaseItemDto, sources []MovieSource) {
	if len(sources) == 0 || sources[0].Probe == nil || sources[0].Probe.Validate() != nil || len(item.MediaSources) == 0 {
		return
	}
	first := item.MediaSources[0]
	item.RunTimeTicks = first.RunTimeTicks
	item.Container, item.Size, item.Bitrate, item.Width, item.Height = first.Container, first.Size, first.Bitrate, first.Width, first.Height
	item.MediaStreams = append(dto.Array[dto.MediaStream]{}, first.MediaStreams...)
}
