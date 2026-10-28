package business

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/filemanager"
	"media-tool/internal/modules/library"
	"path"
	"strings"
)

func safeCandidate(c library.Candidate) b.Candidate {
	return b.Candidate{ID: c.ID, Name: safeText(c.Name), AssetID: c.AssetID, State: c.State, Revision: c.Revision, Size: c.Size}
}
func safeRecognition(v library.Recognition) b.Recognition {
	out := b.Recognition{Candidate: safeCandidate(v.Candidate), Hint: safeHint(v.Hint), Matches: []b.Media{}, FileDigest: v.FileDigest, NeedsConfirmation: v.NeedsConfirmation}
	for _, m := range v.Matches {
		out.Matches = append(out.Matches, safeMedia(m))
	}
	if v.Selected != nil {
		m := safeMedia(*v.Selected)
		out.Selected = &m
	}
	return out
}
func safeRepair(v library.STRMRepairBatch) (b.RepairPlan, error) {
	out := b.RepairPlan{ID: v.ID, Digest: v.Digest, State: v.State, ExpiresAt: v.ExpiresAt, Items: []b.RepairItem{}}
	if len(v.Items) > 100 {
		return out, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, i := range v.Items {
		out.Items = append(out.Items, b.RepairItem{AssetID: i.AssetID, Revision: i.Revision, State: i.State, Digest: i.Digest})
	}
	return out, nil
}
func ids(raw []byte, max int) ([]string, error) {
	var in b.IDs
	if decodeBusiness(raw, &in) != nil || len(in.IDs) < 1 || len(in.IDs) > max {
		return nil, invalidBusiness()
	}
	seen := map[string]bool{}
	for _, id := range in.IDs {
		if !validUUID(id) || seen[id] {
			return nil, invalidBusiness()
		}
		seen[id] = true
	}
	return in.IDs, nil
}
func identity(in b.ConfirmIdentity) library.Confirmation {
	return library.Confirmation{MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season, Episodes: in.Episodes, Revision: in.Revision, MigrateOutputs: in.MigrateOutputs, FileDigest: in.FileDigest, PlanDigest: in.PlanDigest}
}
func (h AcquisitionHost) callLibrary(ctx context.Context, call contracts.HostCall, admit func() error) (any, error) {
	if h.Connections == nil {
		return nil, unsupportedBusiness()
	}
	e := library.Engine{DB: h.DB, Connections: h.Connections, Observe: h.Observe}
	switch call.Method {
	case "library.assets", "strm.entries":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		v, total, err := e.ManagedAssetsKind(ctx, in.Size, (in.Page-1)*in.Size, in.Query, call.Method == "strm.entries")
		if err != nil {
			return nil, err
		}
		out := b.Page[b.Asset]{Items: []b.Asset{}, HasMore: total > in.Page*in.Size}
		for _, a := range v {
			out.Items = append(out.Items, b.Asset{ID: a.ID, Title: safeText(a.Title), MediaType: a.MediaType, Year: a.Year, State: a.ArtifactsState, Revision: a.Revision, Revoked: a.Revoked})
		}
		return out, nil
	case "library.candidates":
		var in b.LibraryPage
		if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, err := e.CandidatePage(ctx, in.Size+1, (in.Page-1)*in.Size, in.State)
		if err != nil {
			return nil, err
		}
		out := b.Page[b.Candidate]{Items: []b.Candidate{}, HasMore: len(v) > in.Size}
		if len(v) > in.Size {
			v = v[:in.Size]
		}
		for _, c := range v {
			out.Items = append(out.Items, safeCandidate(c))
		}
		return out, nil
	case "library.history":
		var in b.LibraryPage
		if decodeBusiness(call.Args, &in) != nil || pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		v, total, err := e.History(ctx, library.HistoryFilter{Kind: in.Kind, State: in.State, Query: in.Query, Limit: in.Size, Offset: (in.Page - 1) * in.Size})
		if err != nil {
			return nil, err
		}
		out := b.Page[b.LibraryHistory]{Items: []b.LibraryHistory{}, HasMore: total > in.Page*in.Size}
		for _, v := range v {
			out.Items = append(out.Items, b.LibraryHistory{ID: v.ID, Kind: v.Kind, State: v.State, Title: safeText(v.Media.Title), UpdatedAt: v.UpdatedAt})
		}
		return out, nil
	case "library.scans":
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		}
		v, err := e.Scans(ctx)
		if err != nil {
			return nil, err
		}
		out := []b.Scan{}
		for _, v := range v {
			out = append(out, b.Scan{ID: v.ID, RootID: v.ConnectionID, State: v.State, Files: v.Files, Directories: v.Directories, Revision: v.Revision})
		}
		return pageSlice(out, in), nil
	case "library.scan.start":
		var in b.ScanStart
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.RootID) {
			return nil, invalidBusiness()
		}
		// Start currently selects the configured primary cloud integration. Never
		// silently scan a different root than the one the plugin asked for.
		conn, err := h.Connections.Repository.Default(ctx, "clouddrive2")
		if err != nil {
			return nil, err
		}
		if conn.Record.ID != in.RootID {
			return nil, unsupportedBusiness()
		}
		root := conn.String("cloudRoot")
		if root == "" && len(h.Connections.Config.STRMAllowedRoots) > 0 {
			root = h.Connections.Config.STRMAllowedRoots[0]
		}
		full, err := relativeCloudRoot(root, in.Path)
		if err != nil {
			return nil, err
		}
		if err = admit(); err != nil {
			return nil, err
		}
		id, err := e.Start(ctx, full)
		return b.Receipt{ID: id, State: "requested"}, err
	case "library.scan.control":
		var in b.Control
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		scan, err := e.ScanByID(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		if call.ExpectedRevision == nil || *call.ExpectedRevision != scan.Revision {
			return nil, contracts.NewError(contracts.Conflict)
		}
		if err = admit(); err != nil {
			return nil, err
		}
		err = e.ControlScan(ctx, in.ID, in.Action)
		return b.Receipt{ID: in.ID, State: "requested"}, err
	case "library.recognize", "library.organize.preview", "library.suggest":
		id, err := businessID(call.Args)
		if err != nil {
			return nil, err
		}
		if call.Method == "library.suggest" {
			v, err := e.Suggest(ctx, id)
			out := []b.Media{}
			if len(v) > 200 {
				return nil, contracts.NewError(contracts.PayloadTooLarge)
			}
			for _, m := range v {
				out = append(out, safeMedia(m))
			}
			return out, err
		}
		v, err := e.Recognize(ctx, id)
		if len(v.Matches) > 200 {
			return nil, contracts.NewError(contracts.PayloadTooLarge)
		}
		return safeRecognition(v), err
	case "library.organize.confirm", "library.correct.preview", "library.correct.apply":
		var in b.ConfirmIdentity
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || in.Revision < 1 || len(in.Episodes) > 100 || call.ExpectedRevision == nil || *call.ExpectedRevision != in.Revision {
			return nil, invalidBusiness()
		}
		if err := admit(); err != nil {
			return nil, err
		}
		if call.Method == "library.correct.preview" {
			v, err := e.PreviewIdentityMigration(ctx, in.ID, identity(in))
			if err != nil {
				return nil, err
			}
			if len(v.Operations) > 200 {
				return nil, contracts.NewError(contracts.PayloadTooLarge)
			}
			out := b.IdentityPlan{ID: v.AssetID, Revision: v.Revision, PlanDigest: v.PlanDigest, Operations: []b.IdentityOperation{}}
			for _, op := range v.Operations {
				out.Operations = append(out.Operations, b.IdentityOperation{Path: safeText(op.Path), Kind: op.Kind, Operation: op.Operation})
			}
			return out, nil
		}
		if call.Method == "library.organize.confirm" {
			if len(in.FileDigest) != 64 {
				return nil, contracts.NewError(contracts.NeedsAction)
			}
			id, err := e.Confirm(ctx, in.ID, identity(in))
			return b.Receipt{ID: id, State: "confirmed"}, err
		}
		if in.MigrateOutputs && len(in.PlanDigest) != 64 {
			return nil, contracts.NewError(contracts.NeedsAction)
		}
		err := e.Correct(ctx, in.ID, identity(in))
		return b.Receipt{ID: in.ID, State: "corrected"}, err
	case "library.refresh":
		id, err := businessID(call.Args)
		if err != nil {
			return nil, err
		}
		if err = admit(); err != nil {
			return nil, err
		}
		err = e.QueueRefresh(ctx, id)
		return b.Receipt{ID: id, State: "requested"}, err
	case "library.history.hide":
		var in b.HistorySelections
		if decodeBusiness(call.Args, &in) != nil || len(in.Items) < 1 || len(in.Items) > 50 {
			return nil, invalidBusiness()
		}
		v := []library.HistorySelection{}
		for _, i := range in.Items {
			if !validUUID(i.ID) {
				return nil, invalidBusiness()
			}
			v = append(v, library.HistorySelection{ID: i.ID, Kind: i.Kind, UpdatedAt: i.UpdatedAt})
		}
		if err := admit(); err != nil {
			return nil, err
		}
		return b.Receipt{State: "hidden"}, e.HideHistory(ctx, v, true)
	case "strm.diagnostics":
		id, err := businessID(call.Args)
		if err != nil {
			return nil, err
		}
		v, err := e.STRMRepairBatch(ctx, id)
		if err != nil {
			return nil, err
		}
		return safeRepair(v)
	case "strm.repair.preview":
		v, err := ids(call.Args, 100)
		if err != nil {
			return nil, err
		}
		if err = admit(); err != nil {
			return nil, err
		}
		plan, err := e.PreviewSTRMRepair(ctx, v)
		if err != nil {
			return nil, err
		}
		return safeRepair(plan)
	case "strm.repair.approve", "strm.repair.retry":
		var in b.Approval
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || len(in.Digest) != 64 {
			return nil, invalidBusiness()
		}
		if err := admit(); err != nil {
			return nil, err
		}
		var v library.STRMRepairBatch
		var err error
		if call.Method == "strm.repair.approve" {
			v, err = e.ApproveSTRMRepair(ctx, in.ID, in.Digest, true)
		} else {
			v, err = e.RetrySTRMRepair(ctx, in.ID, in.Digest, true)
		}
		if err != nil {
			return nil, err
		}
		return safeRepair(v)
	}
	return nil, unsupportedBusiness()
}

// Only a relative path below the configured cloud root may enter Start.
func relativeCloudRoot(root, p string) (string, error) {
	if root == "" || root == "/" || !strings.HasPrefix(root, "/") || path.Clean(root) != root {
		return "", invalidBusiness()
	}
	if _, err := filemanager.Relative(p); err != nil {
		return "", invalidBusiness()
	}
	return path.Join(root, p), nil
}

// A receipt key is not itself a business request UUID. Namespace it by instance
// through the encrypted receipt's unique ID passed by the Host bridge.
func operationUUID(key string) string {
	sum := sha256.Sum256([]byte(key))
	sum[6] = sum[6]&15 | 0x50
	sum[8] = sum[8]&63 | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
