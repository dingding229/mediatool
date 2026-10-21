// Package mediaext is the shared filename-extension policy for ingest and STRM.
// It classifies containers, not codec/browser support or trusted file contents.
package mediaext

import (
	"path"
	"slices"
	"strings"
)

var videos = []string{".mkv", ".mp4", ".m4v", ".mov", ".avi", ".ts", ".m2ts", ".mts", ".webm", ".wmv", ".flv", ".mpg", ".mpeg", ".rmvb", ".vob"}
var subtitles = []string{".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx", ".sup"}

// VideoExtensions and SubtitleExtensions return independent, lowercase lists.
func VideoExtensions() []string    { return slices.Clone(videos) }
func SubtitleExtensions() []string { return slices.Clone(subtitles) }

func extension(name string) string {
	return strings.ToLower(path.Ext(strings.ReplaceAll(name, `\`, "/")))
}
func IsVideo(name string) bool    { return slices.Contains(videos, extension(name)) }
func IsSubtitle(name string) bool { return slices.Contains(subtitles, extension(name)) }

// IsContainer also protects ISO's suffix when renaming; ISO is NOT playable and
// must not enter video ingest/STRM generation. Lavf cannot parse disc images via
// the strict original-STRM -> HTTP 302 delivery path.
func IsContainer(name string) bool { return IsVideo(name) || extension(name) == ".iso" }
