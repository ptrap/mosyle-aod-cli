package aod

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const selfServiceBinary = "/Applications/Self-Service.app/Contents/MacOS/Self-Service"

var errNativeUnsupported = errors.New("native session acquisition is unsupported for this app or authentication mode")

type nativeConfig struct{ account, appKey, libKey string }

// PrepareRequest tries a fresh native session, then saved cookies if native
// authentication or preparation fails. It never submits an elevation request.
// Call only after local authentication.
func (s System) PrepareRequest(ctx context.Context, device, build string) (*Client, Profile, error) {
	return prepareWithFallback(ctx, device, build, func() (*Client, error) {
		cfg, err := s.nativeConfig(ctx)
		if err != nil {
			return nil, err
		}
		c := NewClient(nil)
		if err := c.authenticateNative(ctx, cfg, device, build); err != nil {
			return nil, err
		}
		return c, nil
	}, s.Client)
}

func prepareWithFallback(ctx context.Context, device, build string, native, saved func() (*Client, error)) (*Client, Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, Profile{}, err
	}
	c, nativeErr := native()
	if nativeErr == nil {
		var p Profile
		p, nativeErr = c.Prepare(ctx, device, build)
		if nativeErr == nil {
			return c, p, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, Profile{}, err
	}
	c, err := saved()
	if err != nil {
		// Preserve a useful native failure when there is no usable saved session.
		if !errors.Is(nativeErr, errNativeUnsupported) {
			return nil, Profile{}, nativeErr
		}
		return nil, Profile{}, err
	}
	p, err := c.Prepare(ctx, device, build)
	if err != nil {
		return nil, Profile{}, err
	}
	return c, p, nil
}

func (s System) nativeConfig(ctx context.Context) (nativeConfig, error) {
	var cfg nativeConfig
	f, err := os.Open(selfServiceBinary)
	if err != nil {
		return cfg, errNativeUnsupported
	}
	defer f.Close()
	data, err := readLimited(f, 128<<20)
	if err != nil {
		return cfg, errNativeUnsupported
	}
	cfg.appKey, cfg.libKey, err = nativeConstants(data)
	if err != nil {
		return cfg, err
	}
	path := filepath.Join(s.Home, "Library/Preferences/com.mosyle.macos.business.plist")
	// plutil handles binary and XML plists without exposing any preference values
	// in errors. The dotted key is literal, not a nested key path.
	data, err = command(ctx, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", path)
	if err != nil {
		return cfg, errNativeUnsupported
	}
	cfg.account, err = nativeAccount(plistValue(data, "com.mosyle.macos.business.school.schoolWebviewURL"))
	return cfg, err
}

func nativeAccount(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "mybusiness.mosyle.com" || u.User != nil || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errNativeUnsupported
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q["account"]) != 1 || strings.TrimSpace(q.Get("account")) == "" {
		return "", errNativeUnsupported
	}
	return q.Get("account"), nil
}

func nativeConstants(data []byte) (string, string, error) {
	// These are candidate addresses from the inspected app, not a version gate.
	// A changed layout is rejected locally or by Mosyle and uses saved cookies.
	fat, err := macho.NewFatFile(bytes.NewReader(data))
	if err != nil {
		// App distributions may contain a thin ARM64 executable instead.
		f, thinErr := macho.NewFile(bytes.NewReader(data))
		if thinErr != nil || f.Cpu != macho.CpuArm64 {
			return "", "", errNativeUnsupported
		}
		return nativeFileConstants(f)
	}
	for _, arch := range fat.Arches {
		if arch.Cpu != macho.CpuArm64 {
			continue
		}
		return nativeFileConstants(arch.File)
	}
	return "", "", errNativeUnsupported
}

func nativeFileConstants(f *macho.File) (string, string, error) {
	appKey, err := nativeLiteral(f, 0x100188590, 32)
	if err != nil {
		return "", "", err
	}
	libKey, err := nativeLiteral(f, 0x100189e90, 64)
	return appKey, libKey, err
}

func nativeLiteral(f *macho.File, address uint64, length int) (string, error) {
	for _, load := range f.Loads {
		segment, ok := load.(*macho.Segment)
		if !ok || segment.Name != "__TEXT" || address < segment.Addr {
			continue
		}
		offset := address - segment.Addr
		if offset >= segment.Filesz || uint64(length+1) > segment.Filesz-offset {
			continue
		}
		b := make([]byte, length+1)
		if n, err := segment.ReadAt(b, int64(offset)); err != nil || n != len(b) {
			return "", errNativeUnsupported
		}
		if b[length] != 0 {
			return "", errNativeUnsupported
		}
		for _, ch := range b[:length] {
			if ch < 33 || ch > 126 {
				return "", errNativeUnsupported
			}
		}
		return string(b[:length]), nil
	}
	return "", errNativeUnsupported
}

// stringOrNumber accepts only a nonempty string or JSON number, never objects,
// booleans, or null. Response content must never be included in errors.
func stringOrNumber(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s, true
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil && n.String() != "" {
		return n.String(), true
	}
	return "", false
}

func (c *Client) authenticateNative(ctx context.Context, cfg nativeConfig, device, build string) error {
	if device == "" || build == "" || cfg.account == "" || cfg.appKey == "" || cfg.libKey == "" {
		return ErrProtocol
	}
	fields := url.Values{"deviceudid": {device}, "device_udid": {device}, "has_mdm": {"1"},
		"appkey": {cfg.appKey}, "libKey": {cfg.libKey}, "oswebview": {"mac"}, "build": {build}, "operation": {"device_info"}}
	body, err := c.post(ctx, "/services/", fields, false)
	if err != nil {
		return err
	}
	var result struct {
		Status   string `json:"status"`
		Response struct {
			Device         string          `json:"deviceudid"`
			OS             string          `json:"os"`
			Authentication string          `json:"device_authentication"`
			NeedPassword   json.RawMessage `json:"need_password"`
			User           json.RawMessage `json:"iduser"`
			Token          string          `json:"token"`
		} `json:"response"`
	}
	if json.Unmarshal(body, &result) != nil || result.Status != "OK" {
		return ErrProtocol
	}
	r := result.Response
	if !strings.EqualFold(r.Device, device) || r.OS != "mac" {
		return ErrProtocol
	}
	var auth struct {
		Modes []string `json:"auth_type"`
	}
	if json.Unmarshal([]byte(r.Authentication), &auth) != nil {
		return ErrProtocol
	}
	password, ok := stringOrNumber(r.NeedPassword)
	if !ok {
		return ErrProtocol
	}
	if len(auth.Modes) != 1 || auth.Modes[0] != "NO_AUTH" || password != "0" {
		return errNativeUnsupported
	}
	user, ok := stringOrNumber(r.User)
	id, parseErr := strconv.ParseUint(user, 10, 64)
	if !ok || parseErr != nil || id == 0 || r.Token == "" {
		return ErrProtocol
	}
	fields.Del("operation")
	fields.Set("screen", "SELFSERVICE")
	fields.Set("UIConfiguration", "1")
	fields.Set("DeviceAuthenticationModes", `["NO_AUTH"]`)
	fields.Set("iduser", user)
	fields.Set("token", r.Token)
	query := url.Values{"account": {cfg.account}, "application": {"1"}}
	_, err = c.post(ctx, "/?"+query.Encode(), fields, false)
	return err
}
