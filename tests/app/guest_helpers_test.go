package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
)

// newGuestKey makes a guest the way a script does, and returns its key.
func newGuestKey(t *testing.T, baseURL string) string {
	t.Helper()
	var out struct {
		Key string `json:"key"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/guest", nil, &out); code != 201 || out.Key == "" {
		t.Fatalf("POST /api/guest: %d %+v", code, out)
	}
	return out.Key
}

// browserGuestKey is the guest key of the browser behind ctx: the HttpOnly
// cookie a page cannot read, but the DevTools protocol can. A test that acts
// over the API for a seat the browser took acts as that browser's guest.
func browserGuestKey(t *testing.T, ctx context.Context) string {
	t.Helper()
	var key string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookies {
			if c.Name == "bingle_guest" {
				key = c.Value
			}
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if key == "" {
		t.Fatal("the browser has no guest yet")
	}
	return key
}

// setBrowserGuest makes the browser behind ctx the guest whose key is key,
// for baseURL: what a browser that took a seat earlier carries.
func setBrowserGuest(t *testing.T, ctx context.Context, baseURL, key string) {
	t.Helper()
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookie("bingle_guest", key).WithURL(baseURL).WithPath("/").WithHTTPOnly(true).Do(ctx)
	}))
	if err != nil {
		t.Fatal(err)
	}
}

// newMemberServer is a server and the session cookie of an account signed in
// there (@ana): for what is account-only, such as practice batches.
func newMemberServer(t *testing.T) (string, string) {
	t.Helper()
	_, base := accountServer(t)
	b := newBrowser(t, base)
	if code := b.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	return base, b.cookie(auth.SessionCookie)
}

// doJSONMember is doJSON as the account whose session cookie is session,
// from bingle's page.
func doJSONMember(t *testing.T, session, method, url string, body any, out any) int {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.PageHeader, "1")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: session})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode
}

// signInChrome gives the browser behind ctx the session cookie session.
func signInChrome(t *testing.T, ctx context.Context, baseURL, session string) {
	t.Helper()
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookie(auth.SessionCookie, session).WithURL(baseURL).WithPath("/").WithHTTPOnly(true).Do(ctx)
	}))
	if err != nil {
		t.Fatal(err)
	}
}
