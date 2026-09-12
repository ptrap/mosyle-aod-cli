package aod

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func nativeResponse() map[string]any {
	return map[string]any{"status": "OK", "response": map[string]any{
		"deviceudid": "TEST-DEVICE", "os": "mac", "device_authentication": `{"auth_type":["NO_AUTH"]}`,
		"need_password": 0, "iduser": 123, "token": "synthetic-user-token",
	}}
}

func TestNativeSessionThroughSubmission(t *testing.T) {
	var operations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("expected POST")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		operation := r.Form.Get("operation")
		operations = append(operations, r.URL.Path+":"+operation)
		if r.URL.Path != "/services/" {
			cookie, err := r.Cookie("PHPSESSID")
			if err != nil || cookie.Value != "synthetic-session" {
				t.Error("session cookie missing")
			}
		} else if len(r.Cookies()) != 0 {
			t.Error("native login must start without cookies")
		}
		switch r.URL.Path {
		case "/services/":
			for k, v := range map[string]string{"operation": "device_info", "appkey": "app-key", "libKey": "lib-key", "deviceudid": "TEST-DEVICE", "device_udid": "TEST-DEVICE", "has_mdm": "1", "oswebview": "mac", "build": "182"} {
				if r.PostForm.Get(k) != v {
					t.Errorf("wrong %s", k)
				}
			}
			http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "synthetic-session", Path: "/"})
			json.NewEncoder(w).Encode(nativeResponse())
		case "/":
			for k, v := range map[string]string{"application": "1", "account": "test-account", "screen": "SELFSERVICE", "UIConfiguration": "1", "DeviceAuthenticationModes": `["NO_AUTH"]`, "iduser": "123", "token": "synthetic-user-token"} {
				if r.Form.Get(k) != v {
					t.Errorf("wrong %s", k)
				}
			}
			if r.PostForm.Has("operation") {
				t.Error("device_info operation leaked into webview request")
			}
			w.Write([]byte("<html>Self-Service</html>"))
		case "/screens/scules/selfservice/index.php":
			if r.Form.Has("token") || r.Form.Has("appkey") {
				t.Error("authentication inputs leaked into AOD context")
			}
			w.Write([]byte(fixture))
		case "/Controller/selfservice.php":
			switch operation {
			case "validate_permission_profile":
				w.Write([]byte(`{"ProfileOptions":{"RequestSettings":{"Type":"AllowUsers","RequestMinutes":5}}}`))
			case "validate_ondemand_request":
				if r.Form.Get("justification") != "Test native session" {
					t.Error("missing justification")
				}
				w.Write([]byte(`{"approved":1,"PushEnable":false}`))
			default:
				t.Error("unexpected operation")
			}
		default:
			t.Error("unexpected path")
		}
	}))
	defer server.Close()
	c := NewClient(nil)
	c.origin = server.URL
	if err := c.authenticateNative(context.Background(), nativeConfig{"test-account", "app-key", "lib-key"}, "TEST-DEVICE", "182"); err != nil {
		t.Fatal(err)
	}
	p, err := c.Prepare(context.Background(), "TEST-DEVICE", "182")
	if err != nil || p.Minutes != 5 {
		t.Fatalf("prepare: %v, minutes %d", err, p.Minutes)
	}
	if len(operations) != 4 {
		t.Fatal("authentication or preparation submitted an unexpected operation")
	}
	approval, err := c.Request(context.Background(), p, "Test native session")
	if err != nil || approval.PushEnabled == nil || *approval.PushEnabled {
		t.Fatalf("request: %v", err)
	}
	want := []string{"/services/:device_info", "/:", "/screens/scules/selfservice/index.php:", "/Controller/selfservice.php:validate_permission_profile", "/Controller/selfservice.php:validate_ondemand_request"}
	if !reflect.DeepEqual(operations, want) {
		t.Fatalf("operations %v", operations)
	}
}

func TestNativeRejectsInvalidContext(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
		want   error
	}{
		{"wrong device", func(r map[string]any) { r["deviceudid"] = "another-device" }, ErrProtocol},
		{"wrong OS", func(r map[string]any) { r["os"] = "ios" }, ErrProtocol},
		{"password required", func(r map[string]any) { r["need_password"] = 1 }, errNativeUnsupported},
		{"SSO", func(r map[string]any) { r["device_authentication"] = `{"auth_type":["SSO"]}` }, errNativeUnsupported},
		{"mixed modes", func(r map[string]any) { r["device_authentication"] = `{"auth_type":["NO_AUTH","SSO"]}` }, errNativeUnsupported},
		{"malformed auth", func(r map[string]any) { r["device_authentication"] = "SECRET" }, ErrProtocol},
		{"missing password flag", func(r map[string]any) { delete(r, "need_password") }, ErrProtocol},
		{"missing user", func(r map[string]any) { delete(r, "iduser") }, ErrProtocol},
		{"invalid user", func(r map[string]any) { r["iduser"] = true }, ErrProtocol},
		{"missing token", func(r map[string]any) { delete(r, "token") }, ErrProtocol},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				result := nativeResponse()
				tt.change(result["response"].(map[string]any))
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			c := NewClient(nil)
			c.origin = server.URL
			err := c.authenticateNative(context.Background(), nativeConfig{"test", "app", "lib"}, "TEST-DEVICE", "182")
			if !errors.Is(err, tt.want) || calls != 1 {
				t.Fatalf("err %v, calls %d", err, calls)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("response content exposed")
			}
		})
	}
}

func TestNativeLoginDoesNotRedirectOrRetry(t *testing.T) {
	for _, status := range []int{302, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(status)
			}))
			defer server.Close()
			c := NewClient(nil)
			c.origin = server.URL
			err := c.authenticateNative(context.Background(), nativeConfig{"test", "app", "lib"}, "TEST-DEVICE", "182")
			if err == nil || calls != 1 {
				t.Fatalf("err %v, calls %d", err, calls)
			}
		})
	}
}

func TestNativeAccountAndBinaryRestrictions(t *testing.T) {
	for _, raw := range []string{"http://mybusiness.mosyle.com/?account=x", "https://evil.example/?account=x", "https://mybusiness.mosyle.com.evil.example/?account=x", "https://user@mybusiness.mosyle.com/?account=x", "https://mybusiness.mosyle.com/?account=x&account=y", "https://mybusiness.mosyle.com/?account=x&other=y", "https://mybusiness.mosyle.com/?account=", "https://mybusiness.mosyle.com/path?account=x", "https://mybusiness.mosyle.com/?account=x#fragment"} {
		if _, err := nativeAccount(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if account, err := nativeAccount("https://mybusiness.mosyle.com/?account=test%26account"); err != nil || account != "test&account" {
		t.Fatal("valid selector rejected")
	}
	if _, _, err := nativeConstants([]byte("unrecognized executable")); !errors.Is(err, errNativeUnsupported) {
		t.Fatal("unrecognized executable accepted")
	}
}

// Explicit read-only integration check; never calls Request or changes privileges.
func TestNativeInstalledPreparation(t *testing.T) {
	if os.Getenv("MOSYLE_AOD_NATIVE_LIVE_TEST") != "1" {
		t.Skip("set MOSYLE_AOD_NATIVE_LIVE_TEST=1 for read-only installed-app validation")
	}
	ctx := context.Background()
	s, err := LocalSystem(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device, build, err := s.Device(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := s.nativeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(nil)
	if err := c.authenticateNative(ctx, cfg, device, build); err != nil {
		t.Fatal(err)
	}
	p, err := c.Prepare(ctx, device, build)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Fresh native session validated; policy duration %d minutes; no elevation submitted", p.Minutes)
}

func TestSavedCookieAvailability(t *testing.T) {
	for _, expired := range []bool{false, true} {
		home := t.TempDir()
		path := filepath.Join(home, "Library/HTTPStorages/com.mosyle.macos.business.binarycookies")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, cookieFixture(".mosyle.com", expired), 0600); err != nil {
			t.Fatal(err)
		}
		// The fallback only loads the current user's saved session.
		c, err := (System{Home: home}).Client()
		if expired {
			if !errors.Is(err, ErrSession) {
				t.Fatalf("expired fallback: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(origin)
		cookies := c.http.Jar.Cookies(u)
		if len(cookies) != 1 || cookies[0].Value != "synthetic-cookie" {
			t.Fatal("saved session not loaded")
		}
	}
}

func TestNativeCanceledBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	c := NewClient(nil)
	c.origin = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.authenticateNative(ctx, nativeConfig{"test", "app", "lib"}, "TEST-DEVICE", "182"); err == nil {
		t.Fatal("canceled authentication succeeded")
	}
	if calls != 0 {
		t.Fatal("canceled authentication contacted server")
	}
}

func TestNativeConstantsWithoutVersionPin(t *testing.T) {
	// Minimal synthetic ARM64 Mach-O with the expected string layout.
	data := make([]byte, 0x2100)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(data[off:], v) }
	put64 := func(off int, v uint64) { binary.LittleEndian.PutUint64(data[off:], v) }
	put32(0, 0xfeedfacf)
	put32(4, 0x100000c)
	put32(12, 2)
	put32(16, 1)
	put32(20, 72)
	put32(32, 0x19)
	put32(36, 72)
	copy(data[40:], "__TEXT")
	put64(56, 0x100188000)
	put64(64, 0x2000)
	put64(72, 0x100)
	put64(80, 0x2000)
	copy(data[0x690:], strings.Repeat("A", 32))
	copy(data[0x1f90:], strings.Repeat("B", 64))
	check := func(b []byte) {
		t.Helper()
		a, k, err := nativeConstants(b)
		if err != nil || a != strings.Repeat("A", 32) || k != strings.Repeat("B", 64) {
			t.Fatalf("valid layout: %v", err)
		}
	}
	check(data)
	data[28] = 42 // An unrelated executable change does not block extraction.
	check(data)
	fat := make([]byte, 0x100+len(data))
	binary.BigEndian.PutUint32(fat, 0xcafebabe)
	binary.BigEndian.PutUint32(fat[4:], 1)
	binary.BigEndian.PutUint32(fat[8:], 0x100000c)
	binary.BigEndian.PutUint32(fat[16:], 0x100)
	binary.BigEndian.PutUint32(fat[20:], uint32(len(data)))
	copy(fat[0x100:], data)
	check(fat)
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0x690] = 0 },
		func(b []byte) { b[0x6b0] = 'x' },
		func(b []byte) { b[0x1f90] = 255 },
		func(b []byte) { copy(b[40:], "__DATA") },
	} {
		b := append([]byte(nil), data...)
		mutate(b)
		if _, _, err := nativeConstants(b); err == nil {
			t.Fatal("invalid string layout accepted")
		}
	}
	if _, _, err := nativeConstants(data[:0x1fa0]); err == nil {
		t.Fatal("truncated constants accepted")
	}
}

func TestPreparationFallback(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		nativeErr                     error
		badPage, cancel, missingSaved bool
	}{
		{name: "native success"},
		{name: "unsupported", nativeErr: errNativeUnsupported},
		{name: "network failure", nativeErr: ErrNetwork},
		{name: "authentication rejected", nativeErr: ErrDenied},
		{name: "malformed response", nativeErr: ErrProtocol},
		{name: "login page with HTTP 200", badPage: true},
		{name: "canceled", nativeErr: ErrNetwork, cancel: true},
		{name: "no saved session", nativeErr: ErrNetwork, missingSaved: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				r.ParseForm()
				if r.URL.Path == "/bad/screens/scules/selfservice/index.php" {
					w.Write([]byte("<html>Login</html>"))
					return
				}
				switch r.Form.Get("operation") {
				case "":
					w.Write([]byte(fixture))
				case "validate_permission_profile":
					w.Write([]byte(`{"ProfileOptions":{"RequestSettings":{"Type":"AllowUsers","RequestMinutes":5}}}`))
				default:
					t.Error("preparation must not submit elevation")
				}
			}))
			defer server.Close()
			nativeClient := NewClient(nil)
			nativeClient.origin = server.URL
			if tt.badPage {
				nativeClient.origin += "/bad"
			}
			savedClient := NewClient(nil)
			savedClient.origin = server.URL
			savedCalls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, p, err := prepareWithFallback(ctx, "TEST-DEVICE", "182", func() (*Client, error) {
				if tt.cancel {
					cancel()
				}
				return nativeClient, tt.nativeErr
			}, func() (*Client, error) {
				savedCalls++
				if tt.missingSaved {
					return nil, ErrSession
				}
				return savedClient, nil
			})
			if tt.cancel {
				if !errors.Is(err, context.Canceled) || savedCalls != 0 || requests != 0 {
					t.Fatalf("cancellation: %v, saved %d, HTTP %d", err, savedCalls, requests)
				}
				return
			}
			if tt.missingSaved {
				if !errors.Is(err, ErrNetwork) || savedCalls != 1 {
					t.Fatalf("missing saved: %v", err)
				}
				return
			}
			wantClient := nativeClient
			wantSaved := 0
			if tt.nativeErr != nil || tt.badPage {
				wantClient = savedClient
				wantSaved = 1
			}
			if err != nil || c != wantClient || p.Minutes != 5 || savedCalls != wantSaved {
				t.Fatalf("err %v, minutes %d, saved %d", err, p.Minutes, savedCalls)
			}
		})
	}
}

// Exercises the production fallback boundary, without requesting elevation.
func TestInstalledPreparationWithFallback(t *testing.T) {
	if os.Getenv("MOSYLE_AOD_NATIVE_LIVE_TEST") != "1" {
		t.Skip("opt-in installed-app check")
	}
	ctx := context.Background()
	s, err := LocalSystem(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device, build, err := s.Device(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, p, err := prepareWithFallback(ctx, device, build, func() (*Client, error) {
		cfg, err := s.nativeConfig(ctx)
		if err != nil {
			return nil, err
		}
		c := NewClient(nil)
		err = c.authenticateNative(ctx, cfg, device, build)
		return c, err
	}, func() (*Client, error) { t.Log("Native attempt failed; trying saved session"); return s.Client() })
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Policy validated: %d minutes; no elevation submitted", p.Minutes)
}
