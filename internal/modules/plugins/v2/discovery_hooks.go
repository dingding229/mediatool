package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/dingding229/mediatool-plugin/sdk/events"
	"time"
)

// These are prepare-only policies. Ownership is never accepted from a worker.
func (s Store) ConvertMedia(ctx context.Context, q contracts.MediaConversion) (context.Context, *contracts.MediaIdentity, error) {
	if contracts.ValidateConversion(q, contracts.MediaChoice{Selected: q.Selected}) != nil {
		return ctx, q.Selected, ErrInvalid
	}
	ctx = context.WithValue(ctx, policyBudget{}, time.Now().Add(2*time.Second))
	apply := func(_ context.Context, raw json.RawMessage) bool {
		var r contracts.MediaChoice
		if contracts.Decode(raw, &r) != nil || contracts.ValidateConversion(q, r) != nil {
			return false
		}
		q.Selected = r.Selected
		return true
	}
	ctx, e := s.acquisitionChain(ctx, contracts.ChainMediaRecognizeConvert, &q, apply)
	if e == nil {
		ctx, e = s.policyHooks(ctx, "module", "metadata.identify", &q, apply)
	}
	return ctx, q.Selected, e
}
func (s Store) SupplementEpisodes(ctx context.Context, q contracts.EpisodeExpectation) (context.Context, []int, error) {
	if contracts.ValidateExpectation(q, contracts.EpisodeSupplement{Expected: q.Expected}) != nil {
		return ctx, q.Expected, ErrInvalid
	}
	ctx, e := s.acquisitionChain(ctx, contracts.ChainSubscribeEpisodesRefresh, &q, func(_ context.Context, raw json.RawMessage) bool {
		var r contracts.EpisodeSupplement
		if contracts.Decode(raw, &r) != nil || contracts.ValidateExpectation(q, r) != nil {
			return false
		}
		q.Expected = r.Expected
		return true
	})
	return ctx, q.Expected, e
}
func (s Store) CheckCoverage(ctx context.Context, q contracts.CompletionCheck) (context.Context, bool, error) {
	allow := q.Complete
	ctx, e := s.acquisitionChain(ctx, contracts.ChainSubscribeCompletionCheck, &q, func(_ context.Context, raw json.RawMessage) bool {
		var r contracts.CompletionDecision
		if contracts.Decode(raw, &r) != nil || contracts.ValidateCompletion(q, r) != nil {
			return false
		}
		allow = allow && r.Allow
		q.Complete = allow
		return true
	})
	return ctx, allow, e
}
func validDiscoveryResult(kind, id string, args, result json.RawMessage) bool {
	var payload json.RawMessage = args
	if kind == "chain" {
		var e events.ChainEnvelope
		if contracts.Decode(args, &e) != nil || string(e.Type) != id || e.SchemaVersion != 1 {
			return false
		}
		payload = e.Payload
	}
	switch id {
	case "media.recognize.convert", "metadata.identify":
		var q contracts.MediaConversion
		var r contracts.MediaChoice
		return contracts.Decode(payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateConversion(q, r) == nil
	case "subscribe.episodes.refresh":
		var q contracts.EpisodeExpectation
		var r contracts.EpisodeSupplement
		return contracts.Decode(payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateExpectation(q, r) == nil
	case "subscribe.completion.check":
		var q contracts.CompletionCheck
		var r contracts.CompletionDecision
		return contracts.Decode(payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateCompletion(q, r) == nil
	case "discover.source", "recommend.source":
		var q contracts.SourceRequest
		var r contracts.SourceResult
		return contracts.Decode(payload, &q) == nil && contracts.Decode(result, &r) == nil && contracts.ValidateSource(q, r) == nil
	}
	return false
}
