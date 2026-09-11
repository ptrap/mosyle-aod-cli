package aod

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const origin = "https://mybusiness.mosyle.com"

type Client struct {
	http   *http.Client
	origin string
}
type Profile struct {
	fields  url.Values
	Minutes int
}
type Approval struct {
	PushEnabled *bool `json:"push_enabled,omitempty"`
}

func NewClient(cookies []*http.Cookie) *Client {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(origin)
	jar.SetCookies(u, cookies)
	return &Client{origin: origin, http: &http.Client{Jar: jar, Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrProtocol
	}
	return b, nil
}

func (c *Client) post(ctx context.Context, path string, fields url.Values, submission bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, strings.NewReader(fields.Encode()))
	if err != nil {
		return nil, ErrProtocol
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.origin)
	req.Header.Set("Referer", c.origin+"/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	// No idempotency header and no automatic retries for the request POST.
	r, err := c.http.Do(req)
	if err != nil {
		if submission {
			return nil, ErrUncertain
		}
		return nil, ErrNetwork
	}
	defer r.Body.Close()
	if r.StatusCode == 401 || r.StatusCode >= 300 && r.StatusCode < 400 {
		return nil, ErrSession
	}
	if r.StatusCode == 403 {
		return nil, ErrDenied
	}
	if r.StatusCode != 200 {
		if submission {
			return nil, ErrUncertain
		}
		return nil, ErrProtocol
	}
	b, err := readLimited(r.Body, 2<<20)
	if err != nil {
		if submission {
			return nil, ErrUncertain
		}
		return nil, ErrProtocol
	}
	return b, nil
}

func parseForm(body []byte, device string) (url.Values, error) {
	node, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, ErrProtocol
	}
	var buttons []map[string]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[strings.ToLower(a.Key)] = a.Val
			}
			if strings.Contains(attrs["onclick"], "requestOndemandAdmin(this)") {
				buttons = append(buttons, attrs)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	if len(buttons) != 1 {
		return nil, ErrProtocol
	}
	a := buttons[0]
	for _, k := range []string{"data-deviceudid", "data-profileid", "data-idcompany", "data-integritytoken"} {
		if a[k] == "" {
			return nil, ErrProtocol
		}
	}
	if !strings.EqualFold(a["data-deviceudid"], device) {
		return nil, errors.New("Mosyle returned a form for another device")
	}
	return url.Values{
		"deviceudid": {device}, "ProfileID": {a["data-profileid"]}, "idcompany": {a["data-idcompany"]},
		"IntegrityToken": {a["data-integritytoken"]}, "usertab_current_os": {"mac"}, "usertab_current_idcompany": {a["data-idcompany"]},
	}, nil
}

func (c *Client) Prepare(ctx context.Context, device, build string) (Profile, error) {
	b, err := c.post(ctx, "/screens/scules/selfservice/index.php", url.Values{
		"deviceudid": {device}, "oswebview": {"mac"}, "build": {build}, "HasUpperBar": {"0"}, "tab_opened": {"ON_DEMAND"},
	}, false)
	if err != nil {
		return Profile{}, err
	}
	fields, err := parseForm(b, device)
	if err != nil {
		return Profile{}, err
	}
	fields.Set("operation", "validate_permission_profile")
	fields.Set("mapping", "OnDemandController")
	b, err = c.post(ctx, "/Controller/selfservice.php", fields, false)
	if err != nil {
		return Profile{}, err
	}
	var result struct {
		ProfileOptions struct {
			RequestSettings struct {
				Type           string
				RequestMinutes json.RawMessage
			}
		}
	}
	if json.Unmarshal(b, &result) != nil {
		return Profile{}, ErrProtocol
	}
	settings := result.ProfileOptions.RequestSettings
	if settings.Type == "" {
		return Profile{}, ErrProtocol
	}
	if settings.Type != "AllowUsers" {
		return Profile{}, ErrDenied
	}
	minutes, err := strconv.Atoi(strings.Trim(string(settings.RequestMinutes), `"`))
	if err != nil || minutes <= 0 {
		return Profile{}, ErrProtocol
	}
	fields.Del("mapping")
	return Profile{fields: fields, Minutes: minutes}, nil
}

func (c *Client) Request(ctx context.Context, p Profile, reason string) (Approval, error) {
	if strings.TrimSpace(reason) == "" {
		return Approval{}, errors.New("a justification is required")
	}
	if p.fields == nil {
		return Approval{}, ErrProtocol
	}
	fields := url.Values{}
	for k, v := range p.fields {
		fields[k] = append([]string(nil), v...)
	}
	fields.Set("operation", "validate_ondemand_request")
	fields.Set("UDID", fields.Get("deviceudid"))
	fields.Set("justification", reason)
	b, err := c.post(ctx, "/Controller/selfservice.php", fields, true)
	if err != nil {
		return Approval{}, err
	}
	var result struct {
		Approved   json.RawMessage
		PushEnable *bool
	}
	if json.Unmarshal(b, &result) != nil {
		return Approval{}, ErrUncertain
	}
	approved := string(result.Approved)
	if approved == "0" || approved == `"0"` || approved == "false" {
		return Approval{}, ErrDenied
	}
	if approved != "1" && approved != `"1"` {
		return Approval{}, ErrUncertain
	}
	return Approval{PushEnabled: result.PushEnable}, nil
}
