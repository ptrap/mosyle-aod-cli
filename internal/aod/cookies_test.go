package aod

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func cookieFixture(domain string, expired bool) []byte {
	record := make([]byte, 56)
	for i, s := range []string{domain, "SESSION", "/", "synthetic-cookie"} {
		binary.LittleEndian.PutUint32(record[16+i*4:], uint32(len(record)))
		record = append(record, []byte(s)...)
		record = append(record, 0)
	}
	binary.LittleEndian.PutUint32(record, uint32(len(record)))
	seconds := float64(time.Date(2090, 1, 1, 0, 0, 0, 0, time.UTC).Unix() - 978307200)
	if expired {
		seconds = 1
	}
	binary.LittleEndian.PutUint64(record[40:], math.Float64bits(seconds))
	page := make([]byte, 16)
	page[0] = 0
	page[1] = 0
	page[2] = 1
	page[3] = 0
	binary.LittleEndian.PutUint32(page[4:], 1)
	binary.LittleEndian.PutUint32(page[8:], 16)
	page = append(page, record...)
	data := make([]byte, 12)
	copy(data, "cook")
	binary.BigEndian.PutUint32(data[4:], 1)
	binary.BigEndian.PutUint32(data[8:], uint32(len(page)))
	return append(data, page...)
}
func TestCookies(t *testing.T) {
	for _, tt := range []struct {
		domain  string
		expired bool
		count   int
	}{{".mosyle.com", false, 1}, {"mybusiness.mosyle.com", false, 1}, {"evilmosyle.com", false, 0}, {"mosyle.com.evil.test", false, 0}, {".mosyle.com", true, 0}} {
		c, e := ParseCookies(cookieFixture(tt.domain, tt.expired), time.Now())
		if e != nil || len(c) != tt.count {
			t.Fatalf("%s count=%d err=%v", tt.domain, len(c), e)
		}
	}
}
func TestTruncatedCookies(t *testing.T) {
	valid := cookieFixture(".mosyle.com", false)
	for n := 0; n < len(valid); n++ {
		if _, err := ParseCookies(valid[:n], time.Now()); err == nil {
			t.Fatalf("accepted truncation at %d", n)
		}
	}
	invalid := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(invalid[20:], 0xffffffff)
	if _, err := ParseCookies(invalid, time.Now()); err == nil {
		t.Fatal("accepted invalid offset")
	}
}
func FuzzCookies(f *testing.F) {
	f.Add(cookieFixture(".mosyle.com", false))
	f.Add([]byte("cook"))
	f.Fuzz(func(t *testing.T, b []byte) { ParseCookies(b, time.Now()) })
}
