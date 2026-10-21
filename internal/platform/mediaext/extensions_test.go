package mediaext

import (
	"reflect"
	"strings"
	"testing"
)

func TestSharedExtensionPolicies(t *testing.T) {
	wantVideo := strings.Fields(".mkv .mp4 .m4v .mov .avi .ts .m2ts .mts .webm .wmv .flv .mpg .mpeg .rmvb .vob")
	wantSubtitle := strings.Fields(".srt .ass .ssa .vtt .sub .idx .sup")
	if !reflect.DeepEqual(VideoExtensions(), wantVideo) || !reflect.DeepEqual(SubtitleExtensions(), wantSubtitle) {
		t.Fatal("extension contract changed")
	}
	for _, ext := range wantVideo {
		for _, name := range []string{"Owned" + ext, "/media/Owned" + strings.ToUpper(ext), `C:\media\Owned` + ext} {
			if !IsVideo(name) || !IsContainer(name) || IsSubtitle(name) {
				t.Errorf("video %q misclassified", name)
			}
		}
	}
	for _, ext := range wantSubtitle {
		for _, name := range []string{"Owned.zh-CN" + ext, "/media/Owned" + strings.ToUpper(ext)} {
			if IsVideo(name) || IsContainer(name) || !IsSubtitle(name) {
				t.Errorf("subtitle %q misclassified", name)
			}
		}
	}
	for _, name := range []string{"Owned.iso", "/disc/Owned.ISO"} {
		if IsVideo(name) || !IsContainer(name) || IsSubtitle(name) {
			t.Errorf("ISO %q must only retain rename protection", name)
		}
	}
	for _, name := range []string{"", "mkv", "Owned", "Owned.strm", "Owned.nfo", "Owned.jpg", "Owned.mp3", "Owned.mp4.exe", "/Owned.mkv/notes.txt", "Owned.mkv?download=1"} {
		if IsVideo(name) || IsContainer(name) || IsSubtitle(name) {
			t.Errorf("nonmedia %q accepted", name)
		}
	}
	videosCopy, subtitlesCopy := VideoExtensions(), SubtitleExtensions()
	videosCopy[0], subtitlesCopy[0] = ".iso", ".mkv"
	if !IsVideo("Owned.mkv") || IsVideo("Owned.iso") || !IsSubtitle("Owned.srt") || IsSubtitle("Owned.mkv") {
		t.Fatal("caller mutated shared policy")
	}
}
