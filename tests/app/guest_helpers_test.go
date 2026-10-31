package app_test

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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
