// Package flow exposes observations and next actions, never authorization to
// execute an effect. Server-side revision, lease and approval fences still apply.
package flow

type Status struct {
	Phase      string `json:"phase"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"nextAction"`
	Reason     string `json:"reason"`
}
