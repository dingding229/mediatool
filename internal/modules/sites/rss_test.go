package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func feedClient(t *testing.T, handler http.HandlerFunc) (Client, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return Client{Config: Config{ID: "owned_feed", Domain: server.URL, Schema: "NexusPHP"}, Account: Account{ID: "ACCOUNT-FIXTURE", Endpoint: server.URL}}, server.URL
}

func feedItem(id, extra, link string) string {
	return `<item><title>Owned Movie (2026)</title><guid>` + id + `</guid><link>` + link + `</link>` + extra + `</item>`
}

func TestUNIT3DRSSNumericGUIDKeyedDownloadAndPayloadSize(t *testing.T) {
	// The upstream template (pinned in PT_CONFIG_COMPATIBILITY.md) has no
	// enclosure: GUID is numeric, link is /torrent/download/{id}.{rsskey}.
	key := "FIXTURE-KEY-2026"
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/torrent/download/42."+key {
			fmt.Fprint(w, "owned-torrent-bytes")
			return
		}
		fmt.Fprint(w, `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom"><channel><title>Owned</title><atom:link rel="self" href="https://foreign.invalid/feed?passkey=IGNORED"/>`+feedItem("42", `<contentlength>2147483648</contentlength><description><![CDATA[Owned description]]></description>`, "/torrent/download/42."+key)+`</channel></rss>`)
	})
	items, err := client.RSS(context.Background(), origin+"/rss/1."+key)
	if err != nil || len(items) != 1 {
		t.Fatal("UNIT3D entry missing", len(items), err)
	}
	item := items[0]
	if item.Resource.TorrentID != "42" || item.Resource.Size != 2147483648 || item.Resource.DownloadURL != origin+"/torrent/download/42."+key || item.Resource.DetailsURL != "" || item.Resource.Seeders != -1 || item.Resource.Leechers != -1 || !item.Resource.Partial {
		t.Fatal("UNIT3D resource mapping changed")
	}
	public, _ := json.Marshal(item.Resource)
	// DownloadURL is JSON-private; public detail links must not be the keyed URL.
	if strings.Contains(string(public), key) || strings.Contains(item.Digest, key) {
		t.Fatal("key exposed in public identity")
	}
	raw, err := client.Torrent(context.Background(), "42", item.Resource.DownloadURL)
	if err != nil || string(raw) != "owned-torrent-bytes" {
		t.Fatal("RSS link cannot be used by torrent adapter", err)
	}
	client2 := client
	client2.Account.ID = "ANOTHER-ACCOUNT"
	other, err := client2.RSS(context.Background(), origin+"/rss/1.otherkey")
	if err != nil || other[0].Digest == item.Digest {
		t.Fatal("baseline not account partitioned", err)
	}
}

func TestRSSNexusEnclosureIsPrivateAndAttachmentLengthIsNotPayloadSize(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`+feedItem("opaque-stable-guid", `<enclosure url="/download.php?id=7&amp;passkey=FIXTURE-SECRET" length="9999"/>`, "/details.php?id=7")+`</channel></rss>`)
	})
	items, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(items) != 1 {
		t.Fatal("Nexus feed failed", err)
	}
	resource := items[0].Resource
	if resource.TorrentID != "7" || resource.Size != 0 || resource.DetailsURL != origin+"/details.php?id=7" || resource.DownloadURL != origin+"/download.php?id=7&passkey=FIXTURE-SECRET" {
		t.Fatal("Nexus URL/unknown size contract changed")
	}
}

func TestAtomRSSUsesRedirectedFeedBaseAndXMLBase(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old/feed" {
			http.Redirect(w, r, "/feeds/current/feed.xml", http.StatusFound)
			return
		}
		fmt.Fprint(w, `<feed xmlns="http://www.w3.org/2005/Atom" xml:base="../../"><title>Owned</title><entry xml:base="torrents/"><title>Owned Atom</title><id>tag:owned.invalid,2026:12</id><summary>Owned summary</summary><link rel="self" href="https://foreign.invalid/ignore"/><link rel="alternate" href="12"/><link rel="enclosure" xml:base="../" href="torrent/download/12.FIXTURE-KEY" length="5"/></entry></feed>`)
	})
	items, err := client.RSS(context.Background(), origin+"/old/feed")
	if err != nil || len(items) != 1 || items[0].Resource.TorrentID != "12" || items[0].Resource.DownloadURL != origin+"/torrent/download/12.FIXTURE-KEY" || items[0].Resource.DetailsURL != origin+"/torrents/12" || items[0].Resource.Size != 0 {
		t.Fatal("Atom base/identity mapping failed", err)
	}
}

func TestFeedEmptyIsValidOnlyForRecognizedCompleteRoots(t *testing.T) {
	valid := []string{`<rss><channel/></rss>`, `<rss version="2.0"><channel><title>Empty</title></channel></rss>`, `<feed xmlns="http://www.w3.org/2005/Atom"/>`}
	for _, raw := range valid {
		entries, _, err := parseFeed([]byte(raw))
		if err != nil || len(entries) != 0 {
			t.Fatal("valid empty feed rejected", err)
		}
	}
	invalid := []string{
		`<html><body>Login FIXTURE-SECRET</body></html>`, `<challenge/>`, `<rss/>`, `<rss><channel/><channel/></rss>`,
		`<rss><channel>`, `<rss><channel/></rss><rss><channel/></rss>`, `<rss><channel/></rss>junk`,
		`<!DOCTYPE rss [<!ENTITY x "secret">]><rss><channel/></rss>`, `<feed/>`, `<feed xmlns="urn:foreign"/>`,
		`<rss><item><title>misplaced</title></item><channel/></rss>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><wrapper><entry/></wrapper></feed>`,
	}
	for _, raw := range invalid {
		entries, _, err := parseFeed([]byte(raw))
		if !errors.Is(err, ErrFeed) || len(entries) != 0 || strings.Contains(err.Error(), "FIXTURE-SECRET") {
			t.Fatal("invalid document accepted or error not redacted", err)
		}
	}
}

func TestFeedBoundsAndAllOrNothingIdentityValidation(t *testing.T) {
	invalidItems := []string{
		feedItem("1", "", "/torrent/download/2.FIXTURE-KEY"),
		feedItem("1", "", "https://foreign.invalid/download.php?id=1&amp;passkey=FIXTURE-KEY"),
		feedItem("opaque", "", "/torrents.php"),
		feedItem("1", `<enclosure url="/download.php?id=2"/>`, "/details.php?id=1"),
		feedItem("1", "", "/download.php?id=1&amp;id=1"),
		feedItem("1", "", "/download.php?id=1&amp;tid=2"),
		feedItem("1", "", "/torrent/download/2.FIXTURE-KEY?id=1"),
		feedItem("1", "", "/download.php?id=1#FIXTURE-KEY"),
		feedItem("1", `<contentlength>-1</contentlength>`, "/download.php?id=1"),
		feedItem("1", `<contentlength>9223372036854775808</contentlength>`, "/download.php?id=1"),
		feedItem("1", "", "/torrent/download/1.FIRST-KEY") + feedItem("1", "", "/torrent/download/1.SECOND-KEY"),
		`<item><link>/download.php?id=1</link></item>`,
	}
	for index, item := range invalidItems {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `<rss><channel>`+feedItem("100", "", "/download.php?id=100")+item+`</channel></rss>`)
			})
			items, err := client.RSS(context.Background(), origin+"/feed?passkey=FIXTURE-KEY")
			if !errors.Is(err, ErrFeed) || len(items) != 0 || strings.Contains(err.Error(), "FIXTURE-KEY") {
				t.Fatal("partial/ambiguous result could mutate baseline", err)
			}
		})
	}
	for _, count := range []int{500, 501} {
		raw := `<rss><channel>` + strings.Repeat(feedItem("1", "", "/download.php?id=1"), count) + `</channel></rss>`
		entries, _, err := parseFeed([]byte(raw))
		if count == 500 && (err != nil || len(entries) != count) || count == 501 && !errors.Is(err, ErrFeed) {
			t.Fatal("feed bound changed", count, len(entries), err)
		}
	}
}

func TestFeedDigestSurvivesRSSKeyRotationAndDuplicateEntries(t *testing.T) {
	key := "FIRST-KEY"
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		item := feedItem("77", "", "/torrent/download/77."+key)
		fmt.Fprint(w, `<rss><channel>`+item+item+`</channel></rss>`)
	})
	first, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(first) != 1 {
		t.Fatal("duplicate entry not collapsed", err)
	}
	key = "SECOND-KEY"
	second, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(second) != 1 || first[0].Digest != second[0].Digest || first[0].Resource.DownloadURL == second[0].Resource.DownloadURL {
		t.Fatal("key rotation became a new baseline identity", err)
	}
}

func TestFeedConfiguredPayloadSizeAndPromotionPerEntry(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`+
			feedItem("opaque-guid", `<enclosure url="/download.php?id=8" length="2147483648"/><description>[Free] Owned</description>`, "/details.php?id=8")+
			feedItem("different-guid", `<enclosure url="/download.php?id=9" length="1073741824"/><description>Owned without promotion</description>`, "/details.php?id=9")+`</channel></rss>`)
	})
	client.Config.Requests = map[string]Map{"rss": {"fields": Map{
		"size":                 Map{"selector": "//enclosure/@length", "filters": []any{Map{"name": "byte_size"}}},
		"downloadvolumefactor": Map{"selector": "//description", "filters": []any{Map{"name": "re_search", "args": []any{`\[Free\]`, 0}}, Map{"name": "not_blank", "args": []any{0, ""}}}},
		"pub_date":             Map{"selector": "//pubDate", "filters": []any{Map{"name": "timezone", "args": []any{"+19"}}}},
	}}}
	items, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(items) != 2 || items[0].Resource.Size != 2147483648 || items[1].Resource.Size != 1073741824 || items[0].Resource.DownloadFactor == nil || *items[0].Resource.DownloadFactor != 0 || items[1].Resource.DownloadFactor != nil {
		t.Fatal("configured size/free factor lost, guessed or read from another row", err)
	}
}

func TestFeedNestingLimitIncludesEntryBodies(t *testing.T) {
	for _, depth := range []int{61, 62} {
		raw := `<rss><channel><item><title>Owned</title><link>/download.php?id=1</link>` + strings.Repeat("<wrapper>", depth) + strings.Repeat("</wrapper>", depth) + `</item></channel></rss>`
		_, _, err := parseFeed([]byte(raw))
		if depth == 61 && err != nil || depth == 62 && !errors.Is(err, ErrFeed) {
			t.Fatal("entry subtree bypassed nesting bound", depth, err)
		}
	}
}

func TestFeedGUIDDoesNotOverrideKnownNexusTorrentIdentity(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`+feedItem("1700000123456", "", "/download.php?id=10")+`</channel></rss>`)
	})
	items, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(items) != 1 || items[0].Resource.TorrentID != "10" {
		t.Fatal("opaque GUID treated as torrent ID", err)
	}
}

func TestFeedNamespaceExtensionsCannotReplaceIdentityOrLinks(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss xmlns:ext="urn:foreign"><channel><item><title>Owned title</title><link>/download.php?id=1</link><ext:title>Wrong</ext:title><ext:link>https://foreign.invalid/download.php?id=999</ext:link><ext:guid>999</ext:guid><ext:enclosure url="https://foreign.invalid/download.php?id=999"/></item></channel></rss>`)
	})
	items, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(items) != 1 || items[0].Resource.TorrentID != "1" || items[0].Resource.Title != "Owned title" || items[0].Resource.DownloadURL != origin+"/download.php?id=1" {
		t.Fatal("extension overwrote standard field", err)
	}
}

func TestFeedDuplicateScalarFieldsFailRatherThanUseLastValue(t *testing.T) {
	for _, field := range []string{"title", "guid", "contentlength"} {
		client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `<rss><channel><item><title>Owned</title><guid>1</guid><contentlength>100</contentlength><link>/download.php?id=1</link><`+field+`>999</`+field+`></item></channel></rss>`)
		})
		_, err := client.RSS(context.Background(), origin+"/feed")
		if !errors.Is(err, ErrFeed) {
			t.Fatal("duplicate field silently overwrote first evidence", field, err)
		}
	}
}

func TestAtomXHTMLTitleAndSummaryPreserveVisibleText(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">Owned <em>Movie</em></div></title><summary type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">Owned <strong>summary</strong></div></summary><link rel="enclosure" href="/download.php?id=1"/></entry></feed>`)
	})
	items, err := client.RSS(context.Background(), origin+"/feed")
	if err != nil || len(items) != 1 || items[0].Resource.Title != "Owned Movie" || items[0].Resource.Subtitle != "Owned summary" {
		t.Fatal("nested Atom text was discarded", err)
	}
}

func TestRSSExplicitDisabledRuleCannotBeBypassed(t *testing.T) {
	called := false
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		fmt.Fprint(w, `<rss><channel/></rss>`)
	})
	client.Config.Requests = map[string]Map{"rss": {"disabled": true}}
	_, err := client.RSS(context.Background(), origin+"/feed")
	if !errors.Is(err, ErrConfig) || called {
		t.Fatal("disabled RSS rule executed", err)
	}
}

func TestFeedXMLBaseCannotEscapeConfiguredDomains(t *testing.T) {
	client, origin := feedClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<feed xmlns="http://www.w3.org/2005/Atom" xml:base="https://foreign.invalid/"><entry><title>Owned</title><link rel="enclosure" href="download.php?id=1"/></entry></feed>`)
	})
	_, err := client.RSS(context.Background(), origin+"/feed")
	if !errors.Is(err, ErrFeed) {
		t.Fatal("foreign XML base accepted", err)
	}
}
