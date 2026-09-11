package aod

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fixture = `<html><script>if (1 < 2) {}</script><a onclick="getObj('MDMSelfService').requestOndemandAdmin(this);" data-deviceudid="TEST-DEVICE" data-profileID="12" data-idcompany="example" data-IntegrityToken="synthetic-token">Continue</a></html>`

func TestForm(t *testing.T) {
	for _, tt := range []struct {
		name, body, device string
		ok                 bool
	}{
		{"valid HTML", fixture, "test-device", true},
		{"wrong device", fixture, "different", false},
		{"missing token", strings.ReplaceAll(fixture, `data-IntegrityToken="synthetic-token"`, ""), "TEST-DEVICE", false},
		{"ambiguous", fixture + fixture, "TEST-DEVICE", false},
		{"login page", "<html>Sign in</html>", "TEST-DEVICE", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseForm([]byte(tt.body), tt.device)
			if (err == nil) != tt.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestRequestFlow(t *testing.T) {
	var operations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("expected POST")
		}
		r.ParseForm()
		if r.URL.Path == "/screens/scules/selfservice/index.php" {
			w.Write([]byte(fixture))
			return
		}
		operation := r.Form.Get("operation")
		operations = append(operations, operation)
		if r.Form.Get("IntegrityToken") != "synthetic-token" {
			t.Error("missing token")
		}
		if r.Form.Get("usertab_current_idcompany") != "example" {
			t.Error("wrong company")
		}
		switch operation {
		case "validate_permission_profile":
			w.Write([]byte(`{"ProfileOptions":{"RequestSettings":{"Type":"AllowUsers","RequestMinutes":10}}}`))
		case "validate_ondemand_request":
			if r.Form.Get("justification") != "Install tools & test" || r.Form.Get("UDID") != "TEST-DEVICE" {
				t.Error("wrong request fields")
			}
			w.Write([]byte(`{"approved":1,"PushEnable":false}`))
		default:
			t.Error("unexpected operation")
		}
	}))
	defer server.Close()
	c := NewClient(nil)
	c.origin = server.URL
	p, err := c.Prepare(context.Background(), "TEST-DEVICE", "182")
	if err != nil {
		t.Fatal(err)
	}
	if p.Minutes != 10 {
		t.Fatal("must respect policy duration")
	}
	if len(operations) != 1 {
		t.Fatal("prepare must not submit")
	}
	a, err := c.Request(context.Background(), p, "Install tools & test")
	if err != nil {
		t.Fatal(err)
	}
	if a.PushEnabled == nil || *a.PushEnabled {
		t.Fatal("expected disabled push")
	}
	if len(operations) != 2 {
		t.Fatal("expected exactly one submission")
	}
}

func TestNoRetryOrRedirectOnSubmission(t *testing.T) {
	for _, status := range []int{302, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/other")
				w.WriteHeader(status)
			}))
			defer server.Close()
			c := NewClient(nil)
			c.origin = server.URL
			fields, _ := parseForm([]byte(fixture), "TEST-DEVICE")
			_, err := c.Request(context.Background(), Profile{fields: fields}, "test")
			if err == nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if status == 500 && !errors.Is(err, ErrUncertain) {
				t.Fatal("server failure must preserve uncertainty")
			}
		})
	}
}

func TestDenialNeverSubmits(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(fixture))
			return
		}
		w.Write([]byte(`{"ProfileOptions":{"RequestSettings":{"Type":"Deny","RequestMinutes":5}}}`))
	}))
	defer server.Close()
	c := NewClient(nil)
	c.origin = server.URL
	_, err := c.Prepare(context.Background(), "TEST-DEVICE", "182")
	if !errors.Is(err, ErrDenied) || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
