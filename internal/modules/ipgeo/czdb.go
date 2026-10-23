// Package ipgeo reads the CZ88 community CZDB wire format without external code.
// Format references: CZ88 help/community and tagphi/czdb_searcher_python.
// No real database or key is embedded; fixtures are generated synthetic files.
package ipgeo

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrDatabase = errors.New("CZDB 数据格式、授权密钥或有效期无效")
var le = binary.LittleEndian

const maxDatabase = 256 << 20

type Location struct {
	IP       string `json:"ip"`
	Country  string `json:"country,omitempty"`
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
	ISP      string `json:"isp,omitempty"`
	Label    string `json:"label,omitempty"`
}
type Database struct {
	data, geo                            []byte
	size, first, last, stride, selection int
	expires                              int
}

func Parse(raw []byte, key string, now time.Time) (*Database, error) {
	fail := func() (*Database, error) { return nil, ErrDatabase }
	if len(raw) < 44 || len(raw) > maxDatabase {
		return fail()
	}
	k, e := base64.StdEncoding.DecodeString(key)
	if e != nil {
		return fail()
	}
	block, e := aes.NewCipher(k)
	if e != nil {
		return fail()
	}
	n := int(le.Uint32(raw[8:12]))
	if n < 16 || n > 1024 || n%16 != 0 || 12+n > len(raw) {
		return fail()
	}
	plain := make([]byte, n)
	for i := 0; i < n; i += 16 {
		block.Decrypt(plain[i:i+16], raw[12+i:12+i+16])
	}
	combined := le.Uint32(plain)
	expire := int(combined & 0xfffff)
	today, _ := strconv.Atoi(now.Format("060102"))
	if combined>>20 != le.Uint32(raw[4:8]) || expire < today {
		return fail()
	}
	offset := 12 + n + int(le.Uint32(plain[4:8]))
	if offset < 12+n || offset > len(raw)-17 {
		return fail()
	}
	data := raw[offset:]
	if int(le.Uint32(data[1:5])) != len(data) {
		return fail()
	}
	size := 4
	if data[0]&1 == 1 {
		size = 16
	}
	stride := size*2 + 5
	first, last := int(le.Uint32(data[5:9])), int(le.Uint32(data[13:17]))
	header := int(le.Uint32(data[9:13]))
	if header%20 != 0 || first < 17+header || last < first || (last-first)%stride != 0 || last > len(data)-stride-4 {
		return fail()
	}
	db := &Database{data: data, size: size, first: first, last: last, stride: stride, selection: int(le.Uint32(data[last+stride : last+stride+4])), expires: expire}
	pos := last + stride + 4
	if db.selection != 0 {
		if pos > len(data)-4 {
			return fail()
		}
		length := int(le.Uint32(data[pos : pos+4]))
		pos += 4
		if length < 1 || length > len(data)-pos || pos+length != len(data) {
			return fail()
		}
		db.geo = bytes.Clone(data[pos:])
		for i := range db.geo {
			db.geo[i] ^= k[i%len(k)]
		}
	} else if pos != len(data) {
		return fail()
	}
	var previous []byte
	for i := first; i <= last; i += stride {
		row := data[i : i+stride]
		start, end := row[:size], row[size:2*size]
		ptr, length := int(le.Uint32(row[2*size:2*size+4])), int(row[2*size+4])
		if bytes.Compare(start, end) > 0 || previous != nil && bytes.Compare(previous, start) >= 0 || ptr < 17+header || length < 2 || ptr > first-length {
			return fail()
		}
		previous = end
		if _, e = db.record(data[ptr : ptr+length]); e != nil {
			return fail()
		}
	}
	return db, nil
}
func (d *Database) Lookup(ip netip.Addr) Location {
	out := Location{IP: ip.String()}
	today, _ := strconv.Atoi(time.Now().Format("060102"))
	if d.expires < today {
		return out
	}
	ip = ip.Unmap()
	if ip.Is4() != (d.size == 4) {
		return out
	}
	b := ip.As16()
	query := b[:]
	if d.size == 4 {
		v := ip.As4()
		query = v[:]
	}
	lo, hi := 0, (d.last-d.first)/d.stride
	for lo <= hi {
		m := (lo + hi) / 2
		p := d.first + m*d.stride
		row := d.data[p : p+d.stride]
		if bytes.Compare(query, row[:d.size]) < 0 {
			hi = m - 1
			continue
		}
		if bytes.Compare(query, row[d.size:2*d.size]) > 0 {
			lo = m + 1
			continue
		}
		ptr, n := int(le.Uint32(row[2*d.size:])), int(row[2*d.size+4])
		out, _ = d.record(d.data[ptr : ptr+n])
		out.IP = ip.String()
		return out
	}
	return out
}
func (d *Database) record(data []byte) (Location, error) {
	out := Location{}
	r := message{data: data}
	ptr, e := r.number()
	if e != nil {
		return out, e
	}
	other, e := r.text()
	if e != nil {
		return out, e
	}
	if len(r.data) != 0 {
		return out, ErrDatabase
	}
	fields := []string{}
	if ptr != 0 {
		p, n := int(ptr&0xffffff), int((ptr>>24)&255)
		if p > len(d.geo)-n || n == 0 {
			return out, ErrDatabase
		}
		g := message{data: d.geo[p : p+n]}
		count, e := g.array()
		if e != nil || count > 32 {
			return out, ErrDatabase
		}
		for i := 0; i < count; i++ {
			v, e := g.text()
			if e != nil {
				return out, e
			}
			if d.selection&(1<<(i+1)) != 0 {
				fields = append(fields, cleanField(v))
			}
		}
		if len(g.data) != 0 {
			return out, ErrDatabase
		}
	}
	if len(fields) > 0 {
		out.Country = fields[0]
	}
	if len(fields) > 1 {
		out.Province = fields[1]
	}
	if len(fields) > 2 {
		out.City = fields[2]
	}
	out.ISP = cleanField(other)
	fields = append(fields, out.ISP)
	display := []string{}
	for _, v := range fields {
		if v != "" {
			display = append(display, v)
		}
	}
	out.Label = strings.Join(display, " ")
	return out, nil
}
func cleanField(v string) string {
	v = strings.TrimSpace(v)
	if v == "null" || v == "0" || strings.EqualFold(v, "CZ88.NET") {
		return ""
	}
	return v
}

// Only the MessagePack unsigned integer, UTF-8 string and string-array subset
// specified by CZDB is accepted, with bounded lengths and no recursive decoder.
type message struct{ data []byte }

func (m *message) take(n int) ([]byte, error) {
	if n < 0 || n > len(m.data) {
		return nil, ErrDatabase
	}
	v := m.data[:n]
	m.data = m.data[n:]
	return v, nil
}
func (m *message) number() (uint64, error) {
	b, e := m.take(1)
	if e != nil {
		return 0, e
	}
	if b[0] < 128 {
		return uint64(b[0]), nil
	}
	n := 0
	switch b[0] {
	case 0xcc:
		n = 1
	case 0xcd:
		n = 2
	case 0xce:
		n = 4
	case 0xcf:
		n = 8
	default:
		return 0, ErrDatabase
	}
	b, e = m.take(n)
	if e != nil {
		return 0, e
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, nil
}
func (m *message) text() (string, error) {
	b, e := m.take(1)
	if e != nil {
		return "", e
	}
	n := 0
	switch {
	case b[0]&0xe0 == 0xa0:
		n = int(b[0] & 31)
	case b[0] == 0xd9:
		b, e = m.take(1)
		if e != nil {
			return "", e
		}
		n = int(b[0])
	case b[0] == 0xda:
		b, e = m.take(2)
		if e != nil {
			return "", e
		}
		n = int(binary.BigEndian.Uint16(b))
	default:
		return "", ErrDatabase
	}
	b, e = m.take(n)
	if e != nil || !utf8.Valid(b) {
		return "", ErrDatabase
	}
	return string(b), nil
}
func (m *message) array() (int, error) {
	b, e := m.take(1)
	if e != nil {
		return 0, e
	}
	if b[0]&0xf0 == 0x90 {
		return int(b[0] & 15), nil
	}
	if b[0] == 0xdc {
		b, e = m.take(2)
		if e == nil {
			return int(binary.BigEndian.Uint16(b)), nil
		}
	}
	return 0, fmt.Errorf("%w: array", ErrDatabase)
}

func decodeKey(v string) ([]byte, error) {
	k, e := base64.StdEncoding.DecodeString(v)
	if e != nil {
		return nil, ErrDatabase
	}
	if _, e = aes.NewCipher(k); e != nil {
		return nil, ErrDatabase
	}
	return k, nil
}
