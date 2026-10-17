package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

type OutputContent struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Owner   string `json:"owner"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
}

// Return the existing body, never reconstruct or rotate a STRM grant for viewing.
// Ownership plus current content digest is mandatory; handmade files are not read.
func (e Engine) OutputContent(ctx context.Context, assetID, relative string) (OutputContent, error) {
	out := OutputContent{Path: relative}
	if relative == "" || len(relative) > 2048 || path.IsAbs(relative) || path.Clean(relative) != relative || relative == ".." || strings.HasPrefix(relative, "../") {
		return out, errors.New("invalid output path")
	}
	var expected []byte
	if err := e.DB.QueryRow(ctx, `SELECT artifact_kind,owner_scope,content_digest FROM managed_media_artifacts WHERE asset_id=$1 AND relative_path=$2`, assetID, relative).Scan(&out.Kind, &out.Owner, &expected); err != nil {
		return out, ErrArtifactConflict
	}
	if out.Kind != "strm" && out.Kind != "nfo" {
		return out, errors.New("only owned STRM/NFO text can be viewed")
	}
	fs, err := os.OpenRoot(e.Connections.Config.STRMOutputDir)
	if err != nil {
		return out, err
	}
	defer deferutil.Close(fs, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, err := readRegular(fs, relative)
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(raw)
	if !bytes.Equal(sum[:], expected) {
		return out, ErrArtifactConflict
	}
	if len(raw) > 256<<10 || !utf8.Valid(raw) {
		return out, errors.New("output exceeds text preview limit")
	}
	out.Digest = hex.EncodeToString(sum[:])
	out.Content = string(raw)
	return out, nil
}
