package aod

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
)

var errCookies = errors.New("invalid binary cookie store")

// ParseCookies accepts Apple's binarycookies format and retains only Mosyle cookies.
// Lengths and offsets are untrusted: malformed files must fail rather than panic.
func ParseCookies(data []byte, now time.Time) ([]*http.Cookie, error) {
	if len(data) < 8 || string(data[:4]) != "cook" {
		return nil, errCookies
	}
	count := uint64(binary.BigEndian.Uint32(data[4:8]))
	if count > uint64((len(data)-8)/4) {
		return nil, errCookies
	}
	pos := uint64(8) + 4*count
	var cookies []*http.Cookie
	for i := uint64(0); i < count; i++ {
		size := uint64(binary.BigEndian.Uint32(data[8+i*4 : 12+i*4]))
		if size < 8 || pos > uint64(len(data)) || size > uint64(len(data))-pos {
			return nil, errCookies
		}
		page := data[pos : pos+size]
		pos += size
		n := uint64(binary.LittleEndian.Uint32(page[4:8]))
		if n > uint64((len(page)-8)/4) {
			return nil, errCookies
		}
		for j := uint64(0); j < n; j++ {
			offset := uint64(binary.LittleEndian.Uint32(page[8+j*4 : 12+j*4]))
			if offset > uint64(len(page)) || uint64(len(page))-offset < 56 {
				return nil, errCookies
			}
			size := uint64(binary.LittleEndian.Uint32(page[offset : offset+4]))
			if size < 56 || size > uint64(len(page))-offset {
				return nil, errCookies
			}
			record := page[offset : offset+size]
			read := func(field int) (string, error) {
				start := uint64(binary.LittleEndian.Uint32(record[field : field+4]))
				if start < 56 || start >= uint64(len(record)) {
					return "", errCookies
				}
				end := bytes.IndexByte(record[start:], 0)
				if end < 0 {
					return "", errCookies
				}
				return string(record[start : start+uint64(end)]), nil
			}
			fields := make([]string, 4)
			for k, field := range []int{16, 20, 24, 28} {
				v, err := read(field)
				if err != nil {
					return nil, err
				}
				fields[k] = v
			}
			domain := strings.ToLower(strings.TrimPrefix(fields[0], "."))
			if domain != "mosyle.com" && domain != "mybusiness.mosyle.com" {
				continue
			}
			seconds := math.Float64frombits(binary.LittleEndian.Uint64(record[40:48]))
			if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 1e12 {
				return nil, errCookies
			}
			expiry := time.Unix(int64(seconds)+978307200, 0)
			if !expiry.After(now) {
				continue
			}
			flags := binary.LittleEndian.Uint32(record[8:12])
			c := &http.Cookie{Name: fields[1], Value: fields[3], Path: fields[2], Domain: fields[0], Expires: expiry, Secure: flags&1 != 0, HttpOnly: flags&4 != 0}
			if c.Valid() != nil {
				return nil, errCookies
			}
			cookies = append(cookies, c)
		}
	}
	return cookies, nil
}
