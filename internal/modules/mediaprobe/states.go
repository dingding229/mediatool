// Package mediaprobe owns the persistent probe and episode-still states.
package mediaprobe

type ProbeState string

const (
	ProbePending     ProbeState = "pending"
	ProbeProbing     ProbeState = "probing"
	ProbeReady       ProbeState = "ready"
	ProbeFailed      ProbeState = "failed"
	ProbeNeedsAction ProbeState = "needs_action"
)

type StillState string

const (
	StillPending     StillState = "pending"
	StillGrabbing    StillState = "grabbing"
	StillReady       StillState = "ready"
	StillFailed      StillState = "failed"
	StillSkipped     StillState = "skipped"
	StillNeedsAction StillState = "needs_action"
)
