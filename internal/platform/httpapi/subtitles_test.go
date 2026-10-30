package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

func subtitleMultipart(t *testing.T, fields [][2]string) (*httptest.ResponseRecorder, *bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	p, e := form.CreateFormFile("file", "sample.chs.srt")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\n这是字幕\n")); e != nil {
		t.Fatal(e)
	}
	for _, f := range fields {
		if e = form.WriteField(f[0], f[1]); e != nil {
			t.Fatal(e)
		}
	}
	if e = form.Close(); e != nil {
		t.Fatal(e)
	}
	return httptest.NewRecorder(), &body, form.FormDataContentType()
}
func TestI40IdentifySubtitleIsReadOnlyAndReturnsLanguageEvidence(t *testing.T) {
	w, body, content := subtitleMultipart(t, [][2]string{{"encoding", "auto"}})
	r := httptest.NewRequest("POST", "/api/v1/media/subtitles/identify", body)
	r.Header.Set("Content-Type", content)
	a := &API{}
	a.identifySubtitle(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var v struct {
		Data struct {
			Language        string
			LanguageTags    []string
			Label, Evidence string
		}
	}
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	if v.Data.Language != "chi" || v.Data.Label != "简体中文" || len(v.Data.LanguageTags) != 1 || v.Data.Evidence == "" {
		t.Fatal(w.Body.String())
	}
}
func TestI40SubtitleLanguageMultipartBoundaries(t *testing.T) {
	for _, c := range []struct {
		name             string
		fields           [][2]string
		selection, valid bool
	}{
		{"manual language", [][2]string{{"encoding", "auto"}, {"uploadId", "id"}, {"language", "chi"}, {"languageTags", `["zh-Hant","en"]`}}, true, true},
		{"duplicate language", [][2]string{{"language", "chi"}, {"language", "eng"}}, true, false},
		{"invalid tag", [][2]string{{"language", "chi"}, {"languageTags", `["bad\nvalue"]`}}, true, false},
		{"tags without language", [][2]string{{"languageTags", `["en"]`}}, true, false},
		{"identify cannot mutate metadata", [][2]string{{"language", "chi"}}, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, body, content := subtitleMultipart(t, c.fields)
			r := httptest.NewRequest("POST", "/", body)
			r.Header.Set("Content-Type", content)
			in, e := readSubtitleUpload(w, r, c.selection)
			if (e == nil) != c.valid {
				t.Fatal(e)
			}
			if c.valid && (in.language.Language != "chi" || in.language.LanguageTags[0] != "zh-Hant") {
				t.Fatal(in)
			}
		})
	}
}
