package sites

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html/charset"
)

// This fixed error is safe for job logs. Never embed feed URLs, keys or raw XML.
var ErrFeed = errors.New("invalid or unsupported RSS/Atom feed; check login and feed compatibility")

const atomNamespace = "http://www.w3.org/2005/Atom"
const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

type FeedItem struct {
	Digest   string
	Resource Resource
}
type feedLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Length string `xml:"length,attr"`
	Base   string `xml:"http://www.w3.org/XML/1998/namespace base,attr"`
	Text   string `xml:",chardata"`
}
type feedEnclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
}
type feedEntry struct {
	XMLName       xml.Name
	Title         string          `xml:"title"`
	GUID          string          `xml:"guid"`
	ID            string          `xml:"id"`
	Description   string          `xml:"description"`
	Summary       string          `xml:"summary"`
	ContentLength string          `xml:"contentlength"`
	Base          string          `xml:"http://www.w3.org/XML/1998/namespace base,attr"`
	Enclosures    []feedEnclosure `xml:"enclosure"`
	Links         []feedLink      `xml:"link"`
}

// RSS and Atom are extensible. Foreign-namespace elements must not replace a
// standard title, GUID or link merely because their local names happen to match.
func (e *feedEntry) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	*e = feedEntry{XMLName: start.Name}
	for _, attr := range start.Attr {
		if attr.Name.Space == xmlNamespace && attr.Name.Local == "base" {
			e.Base = attr.Value
		}
	}
	seen := map[string]bool{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return ErrFeed
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != start.Name.Space {
				if decoder.Skip() != nil {
					return ErrFeed
				}
				continue
			}
			var field *string
			switch token.Name.Local {
			case "title":
				field = &e.Title
			case "guid":
				field = &e.GUID
			case "id":
				field = &e.ID
			case "description":
				field = &e.Description
			case "summary":
				field = &e.Summary
			case "contentlength":
				field = &e.ContentLength
			case "link":
				var link feedLink
				if decoder.DecodeElement(&link, &token) != nil {
					return ErrFeed
				}
				e.Links = append(e.Links, link)
				continue
			case "enclosure":
				var enclosure feedEnclosure
				if decoder.DecodeElement(&enclosure, &token) != nil {
					return ErrFeed
				}
				e.Enclosures = append(e.Enclosures, enclosure)
				continue
			}
			if field == nil {
				if decoder.Skip() != nil {
					return ErrFeed
				}
				continue
			}
			if seen[token.Name.Local] {
				return ErrFeed
			}
			value, err := feedText(decoder)
			if err != nil {
				return err
			}
			*field = value
			seen[token.Name.Local] = true
		case xml.EndElement:
			if token.Name == start.Name {
				return nil
			}
		}
	}
}

// Atom text constructs can contain XHTML. Preserve their visible text rather
// than dropping nested spans/divs or treating attributes as resource URLs.
func feedText(decoder *xml.Decoder) (string, error) {
	depth := 1
	var out strings.Builder
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", ErrFeed
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			out.Write(token)
		}
	}
	return out.String(), nil
}

// Validate the entire token stream before entry decoding. The entry decoder
// cannot skip depth/count/root validation by consuming an untrusted subtree.
func validateFeed(raw []byte) (xml.Name, string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.CharsetReader = charset.NewReaderLabel
	var stack []xml.Name
	var root xml.Name
	seen, closed, channels, entries := false, false, 0, 0
	base := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xml.Name{}, "", ErrFeed
		}
		switch token := token.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				if seen || closed || !((token.Name.Local == "rss" && token.Name.Space == "") || (token.Name.Local == "feed" && token.Name.Space == atomNamespace)) {
					return xml.Name{}, "", ErrFeed
				}
				seen, root = true, token.Name
				for _, attr := range token.Attr {
					if attr.Name.Space == xmlNamespace && attr.Name.Local == "base" {
						base = attr.Value
					}
				}
			} else {
				if root.Local == "rss" && token.Name == (xml.Name{Local: "channel"}) {
					channels++
					if len(stack) != 1 || channels != 1 {
						return xml.Name{}, "", ErrFeed
					}
				}
				isRSSItem := root.Local == "rss" && token.Name == (xml.Name{Local: "item"})
				isAtomEntry := root.Local == "feed" && token.Name == (xml.Name{Space: atomNamespace, Local: "entry"})
				if isRSSItem || isAtomEntry {
					validScope := isRSSItem && len(stack) == 2 && stack[1] == (xml.Name{Local: "channel"}) || isAtomEntry && len(stack) == 1
					entries++
					if !validScope || entries > 500 {
						return xml.Name{}, "", ErrFeed
					}
				}
			}
			stack = append(stack, token.Name)
			if len(stack) > 64 {
				return xml.Name{}, "", ErrFeed
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return xml.Name{}, "", ErrFeed
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				closed = true
			}
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(token)) != "" {
				return xml.Name{}, "", ErrFeed
			}
		case xml.Directive:
			return xml.Name{}, "", ErrFeed
		}
	}
	if !seen || !closed || len(stack) != 0 || root.Local == "rss" && channels != 1 {
		return xml.Name{}, "", ErrFeed
	}
	return root, base, nil
}

func parseFeed(raw []byte) ([]feedEntry, string, error) {
	root, base, err := validateFeed(raw)
	if err != nil {
		return nil, "", err
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.CharsetReader = charset.NewReaderLabel
	entries := []feedEntry{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return entries, base, nil
		}
		if err != nil {
			return nil, "", ErrFeed
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if root.Local == "rss" && start.Name == (xml.Name{Local: "item"}) || root.Local == "feed" && start.Name == (xml.Name{Space: atomNamespace, Local: "entry"}) {
			var entry feedEntry
			if decoder.DecodeElement(&entry, &start) != nil {
				return nil, "", ErrFeed
			}
			entries = append(entries, entry)
		}
	}
}

func (c Client) feedURL(base *url.URL, raw string) (*url.URL, error) {
	if strings.ContainsAny(raw, "\r\n\x00") {
		return nil, ErrFeed
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || base == nil {
		return nil, ErrFeed
	}
	u = base.ResolveReference(u)
	if !c.allowed(u) || u.Fragment != "" {
		return nil, ErrFeed
	}
	return u, nil
}

func (c Client) RSS(ctx context.Context, address string) ([]FeedItem, error) {
	if rule := c.Config.Requests["rss"]; len(rule) > 0 {
		// The explicit feed URL supplies transport, not permission to ignore
		// browser/disabled/parser declarations on the selected definition.
		if err := validateRequestExecution(rule); err != nil {
			return nil, err
		}
	}
	target, err := url.Parse(address)
	if err != nil || !c.allowed(target) || target.Fragment != "" || strings.ContainsAny(address, "\r\n\x00") {
		return nil, ErrFeed
	}
	raw, _, err := c.callWithTarget(ctx, Map{"path": address, "method": "GET", "parser": "None"}, nil, nil, &target)
	if err != nil {
		return nil, err
	}
	entries, feedBase, err := parseFeed(raw)
	if err != nil {
		return nil, err
	}
	if feedBase != "" {
		target, err = c.feedURL(target, feedBase)
		if err != nil {
			return nil, err
		}
	}
	out := []FeedItem{}
	seen := map[string]string{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.Title) == "" {
			return nil, ErrFeed
		}
		entryBase := target
		if entry.Base != "" {
			entryBase, err = c.feedURL(entryBase, entry.Base)
			if err != nil {
				return nil, err
			}
		}
		id, download, detail := "", "", ""
		unit3D := false
		mergeID := func(candidate string) error {
			if candidate != "" {
				if id != "" && candidate != id {
					return ErrFeed
				}
				id = candidate
			}
			return nil
		}
		addLink := func(raw, base string, enclosure bool) error {
			if strings.TrimSpace(raw) == "" {
				return ErrFeed
			}
			linkBase := entryBase
			if base != "" {
				linkBase, err = c.feedURL(linkBase, base)
				if err != nil {
					return err
				}
			}
			u, err := c.feedURL(linkBase, raw)
			if err != nil {
				return err
			}
			candidate, isDownload, err := feedLinkIdentity(u)
			unit3D = unit3D || feedRoutes[0].pattern.MatchString(u.Path)
			if err != nil || mergeID(candidate) != nil {
				return ErrFeed
			}
			if enclosure || isDownload {
				if download != "" && download != u.String() {
					return ErrFeed
				}
				download = u.String()
			} else if detail == "" && feedDetailsPath(u.Path) {
				detail = u.String()
			}
			return nil
		}
		for _, enclosure := range entry.Enclosures {
			if err := addLink(enclosure.URL, "", true); err != nil {
				return nil, err
			}
		}
		for _, link := range entry.Links {
			if link.Rel != "" && link.Rel != "alternate" && link.Rel != "enclosure" {
				continue // Atom self/license/related are not torrent targets.
			}
			raw := link.Href
			if raw == "" {
				raw = link.Text
			}
			if err := addLink(raw, link.Base, link.Rel == "enclosure"); err != nil {
				return nil, err
			}
		}
		// A numeric GUID is corroboration, not a URL credential or standalone
		// permission to download. UNIT3D supplies it together with its keyed link.
		for _, opaque := range []string{entry.GUID, entry.ID} {
			candidate := strings.TrimSpace(opaque)
			if feedIDPattern.MatchString(candidate) && (id == "" || unit3D) {
				if mergeID(candidate) != nil {
					return nil, ErrFeed
				}
			} else if u, parseErr := url.Parse(candidate); parseErr == nil && (u.Scheme == "http" || u.Scheme == "https") {
				if !c.allowed(u) || u.Fragment != "" {
					continue // Opaque/foreign GUIDs are not torrent identities.
				}
				if id == "" {
					candidateID, _, identityErr := feedLinkIdentity(u)
					if identityErr != nil || mergeID(candidateID) != nil {
						return nil, ErrFeed
					}
				}
			}
		}
		if id == "" || (download == "" && c.Config.Schema != "mTorrent") {
			return nil, ErrFeed
		}
		if previous, exists := seen[id]; exists {
			if previous != download {
				return nil, ErrFeed
			}
			continue
		}
		seen[id] = download
		size := int64(0)
		if strings.TrimSpace(entry.ContentLength) != "" {
			var valid bool
			size, valid = checkedInteger(strings.TrimSpace(entry.ContentLength), 63)
			if !valid {
				return nil, ErrFeed
			}
		}
		// Preserve explicitly configured PT size/promotion semantics. The common
		// NexusPHP rule declares enclosure.length as its reported payload size;
		// without that declaration an attachment length is not payload evidence.
		metadata, err := c.feedMetadata(entry)
		if err != nil {
			return nil, err
		}
		if value := metadata["size"]; !blank(value) {
			var valid bool
			size, valid = checkedInteger(value, 63)
			if !valid {
				return nil, ErrFeed
			}
		}
		digest := sha256.Sum256([]byte(c.Account.ID + ":" + id))
		description := entry.Description
		if description == "" {
			description = entry.Summary
		}
		metadata["id"], metadata["title"], metadata["size"] = id, strings.TrimSpace(entry.Title), size
		resource, ok := c.resource(metadata)
		if !ok {
			return nil, ErrFeed
		}
		resource.Partial, resource.Seeders, resource.Leechers = true, -1, -1
		resource.Subtitle, resource.DownloadURL, resource.DetailsURL = description, download, detail
		out = append(out, FeedItem{Digest: hex.EncodeToString(digest[:]), Resource: resource})
	}
	return out, nil
}

// Metadata filters run on a single normalized entry, never on the whole feed.
// This preserves NexusPHP's declared size/free/H&R rules without guessing the
// first digits of a hostname, invoking unused timezone fields, or copying keys.
func (c Client) feedMetadata(entry feedEntry) (Map, error) {
	fields := object(c.Config.Requests["rss"]["fields"])
	if len(fields) == 0 {
		return Map{}, nil
	}
	raw, err := xml.Marshal(entry)
	if err != nil {
		return nil, ErrFeed
	}
	d, err := parseDocument(raw, "Xml")
	if err != nil {
		return nil, err
	}
	return mapFieldsSelected(d, fields, d.xml, c.vars("", 1), []string{"size", "hr", "hr_days", "downloadvolumefactor", "labels"}, nil)
}
