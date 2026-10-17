package probequeue

import "media-tool/internal/modules/mediaprobe/probeinfo"

// ResultParser replaces the duration-only placeholder without changing Engine's
// injected parser contract. Upstream decoding errors never carry raw values.
type ResultParser struct{}

func (ResultParser) Parse(raw []byte) (Result, error) {
	result, err := probeinfo.Parse(raw)
	if err != nil {
		return Result{}, ErrInvalid
	}
	return result, nil
}
