package business

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	b "github.com/dingding229/mediatool-plugin/sdk/host/business"
	"media-tool/internal/modules/filemanager"
	"strings"
)

func rootAllowed(p contracts.Permission, id string) bool {
	return selected(p, "rootIds", id) && selected(p, "storageIds", id)
}
func safeFile(f filemanager.File) b.MediaFile {
	return b.MediaFile{Modified: f.Modified, ID: f.ID, Name: safeText(f.Name), Path: safeText(f.Path), Size: f.Size, Directory: f.Directory, Unsupported: f.Unsupported}
}
func filePlan(r filemanager.Result) (b.FilePlan, error) {
	out := b.FilePlan{ID: r.ID, RootID: r.Plan.ConnectionID, Revision: r.Plan.Revision, Operation: r.Plan.Operation, State: r.State, Digest: r.Plan.Digest, ExpiresAt: r.Plan.ExpiresAt, CompletedSteps: r.CompletedSteps, Items: []b.MediaFile{}, Targets: []string{}}
	if len(r.Plan.Items) > 200 || len(r.Plan.Steps) > 200 {
		return out, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, step := range r.Plan.Steps {
		out.Targets = append(out.Targets, safeText(step.Target))
	}
	for _, i := range r.Plan.Items {
		out.Items = append(out.Items, safeFile(i.Source))
	}
	return out, nil
}
func fileBatch(v filemanager.Batch) (b.FileBatch, error) {
	out := b.FileBatch{ID: v.ID, Digest: v.Digest, State: v.State, Items: []b.FilePlan{}}
	if len(v.Items) > 20 {
		return out, contracts.NewError(contracts.PayloadTooLarge)
	}
	for _, i := range v.Items {
		if i.Result == nil {
			out.Items = append(out.Items, b.FilePlan{State: i.State})
			continue
		}
		p, e := filePlan(*i.Result)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}
func fileInput(in b.FileInput) (filemanager.Input, error) {
	if !validUUID(in.RootID) {
		return filemanager.Input{}, invalidBusiness()
	}
	for _, p := range []string{in.Source, in.Destination} {
		if _, e := filemanager.Relative(p); e != nil {
			return filemanager.Input{}, invalidBusiness()
		}
	}
	switch in.Operation {
	case "organize", "move", "rename", "mkdir", "trash":
	default:
		return filemanager.Input{}, unsupportedBusiness()
	}
	return filemanager.Input{ExpectedModified: in.ExpectedModified, ConnectionID: in.RootID, Operation: in.Operation, Source: in.Source, Destination: in.Destination, Name: in.Name, ExpectedFileID: in.ExpectedFileID, ExpectedSize: in.ExpectedSize}, nil
}
func (h AcquisitionHost) callFiles(ctx context.Context, call contracts.HostCall, p contracts.Permission, admit func() error) (any, error) {
	if h.Connections == nil {
		return nil, unsupportedBusiness()
	}
	e := filemanager.Engine{DB: h.DB, Connections: h.Connections}
	method := strings.TrimPrefix(strings.TrimPrefix(call.Method, "fs.media."), "storage.")
	if method == "roots" {
		var in b.PageRequest
		if decodeBusiness(call.Args, &in) != nil || pageOK(in) != nil {
			return nil, invalidBusiness()
		}
		roots, err := e.Roots(ctx)
		if err != nil {
			return nil, err
		}
		out := []b.Root{}
		for _, r := range roots {
			if rootAllowed(p, r.ID) && (in.Query == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(in.Query))) {
				out = append(out, b.Root{ID: r.ID, Name: safeText(r.Name), Kind: r.Kind, Revision: r.Revision, Read: r.Capabilities.Read, Write: r.Capabilities.Write})
			}
		}
		return pageSlice(out, in), nil
	}
	if method == "list" || method == "stat" {
		var in b.FileQuery
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.RootID) || method == "list" && pageOK(in.PageRequest) != nil {
			return nil, invalidBusiness()
		}
		if !rootAllowed(p, in.RootID) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		if _, err := filemanager.Relative(in.Path); err != nil {
			return nil, invalidBusiness()
		}
		if method == "stat" {
			f, err := e.Stat(ctx, in.RootID, in.Path)
			return safeFile(f), err
		}
		if in.Query != "" {
			return nil, unsupportedBusiness()
		} // no post-page filtering pretending completeness
		raw, err := e.List(ctx, in.RootID, in.Path, in.Size, (in.Page-1)*in.Size)
		if err != nil {
			return nil, err
		}
		out := b.FilePage{RootID: in.RootID, Page: b.Page[b.MediaFile]{Items: []b.MediaFile{}}}
		for _, f := range raw["items"].([]filemanager.File) {
			out.Items = append(out.Items, safeFile(f))
		}
		out.HasMore = raw["total"].(int) > in.Page*in.Size
		return out, nil
	}
	if method == "get" || method == "apply" {
		var in b.Approval
		if method == "get" {
			var id b.ID
			if decodeBusiness(call.Args, &id) != nil {
				return nil, invalidBusiness()
			}
			in.ID = id.ID
		} else if decodeBusiness(call.Args, &in) != nil || len(in.Digest) != 64 {
			return nil, invalidBusiness()
		}
		if !validUUID(in.ID) {
			return nil, invalidBusiness()
		}
		r, err := e.Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		if !rootAllowed(p, r.Plan.ConnectionID) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		if _, err = filePlan(r); err != nil {
			return nil, err
		}
		if method == "apply" {
			if call.ExpectedRevision == nil || *call.ExpectedRevision != r.Plan.Revision {
				return nil, contracts.NewError(contracts.Conflict)
			}
			if err = admit(); err != nil {
				return nil, err
			}
			r, err = e.Execute(ctx, in.ID, in.Digest, true)
			if err != nil {
				return nil, err
			}
		}
		return filePlan(r)
	}
	if method == "approveBatch" {
		var in b.Approval
		if decodeBusiness(call.Args, &in) != nil || !validUUID(in.ID) || len(in.Digest) != 64 {
			return nil, invalidBusiness()
		}
		batch, err := e.Batch(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		if len(batch.Items) > 20 {
			return nil, invalidBusiness()
		}
		for _, i := range batch.Items {
			if i.Result != nil {
				if !rootAllowed(p, i.Result.Plan.ConnectionID) {
					return nil, contracts.NewError(contracts.PermissionDenied)
				}
				if _, err = filePlan(*i.Result); err != nil {
					return nil, err
				}
			}
		}
		if err = admit(); err != nil {
			return nil, err
		}
		batch, err = e.ApproveBatch(ctx, in.ID, in.Digest, true)
		if err != nil {
			return nil, err
		}
		return fileBatch(batch)
	}
	inputs := []b.FileInput{}
	if method == "previewBatch" {
		var in b.FileInputs
		if decodeBusiness(call.Args, &in) != nil || len(in.Items) < 1 || len(in.Items) > 20 {
			return nil, invalidBusiness()
		}
		inputs = in.Items
	} else {
		var in b.FileInput
		if decodeBusiness(call.Args, &in) != nil {
			return nil, invalidBusiness()
		}
		switch method {
		case "mkdir":
			in.Operation = "mkdir"
		case "relocate":
			in.Operation = "move"
		case "preview":
		default:
			return nil, unsupportedBusiness()
		}
		inputs = append(inputs, in)
	}
	native := []filemanager.Input{}
	for _, in := range inputs {
		v, err := fileInput(in)
		if err != nil {
			return nil, err
		}
		if !rootAllowed(p, in.RootID) {
			return nil, contracts.NewError(contracts.PermissionDenied)
		}
		native = append(native, v)
	}
	if err := admit(); err != nil {
		return nil, err
	}
	if method == "previewBatch" {
		batch, err := e.PreviewBatch(ctx, native)
		if err != nil {
			return nil, err
		}
		return fileBatch(batch)
	}
	plan, err := e.Preview(ctx, native[0])
	if err != nil {
		return nil, err
	}
	return filePlan(filemanager.Result{ID: plan.ID, State: "preview", Plan: plan})
}

// The media/file contract is intentionally narrower than the native admin API.
// Enforce the 200-element/rune ceiling recursively, not just at the outer page.
func mediaBounds(raw json.RawMessage) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	var valid func(any, int) bool
	valid = func(v any, depth int) bool {
		if depth > 16 {
			return false
		}
		switch x := v.(type) {
		case string:
			return len([]rune(x)) <= 200 && !strings.ContainsAny(x, "\x00\r\n")
		case []any:
			if len(x) > 200 {
				return false
			}
			for _, y := range x {
				if !valid(y, depth+1) {
					return false
				}
			}
		case map[string]any:
			if len(x) > 200 {
				return false
			}
			for _, y := range x {
				if !valid(y, depth+1) {
					return false
				}
			}
		}
		return true
	}
	return valid(v, 0)
}
