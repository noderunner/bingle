package app_test

// The limits on batches through the REST route and MCP (ADR 0161): a spec over
// a ceiling is refused with the limit in the answer, a player with no room
// left with 429, and only an administrator's own session may raise a batch's
// limits.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
)

// limitedServer is a server with the batch limits given, the account @ana
// (her session cookie) and the administrator's page session.
func limitedServer(t *testing.T, limits config.BatchLimits) (base, ana string, admin *browser) {
	t.Helper()
	base, ana, admin, _ = limitedServerAt(t, limits)
	return base, ana, admin
}

// limitedServerAt is limitedServer, and the path of its database.
func limitedServerAt(t *testing.T, limits config.BatchLimits) (base, ana string, admin *browser, dbPath string) {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.Addr = "127.0.0.1:0" // the seeded administrator needs no second factor here
	cfg.Batch = limits
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	given, err := a.Accounts().Create(context.Background(), "ana", "ana@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, srv.URL)
	if code := b.login("ana", given); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": given, "new": anaPassword}, nil); code != 200 {
		t.Fatalf("password: %d", code)
	}
	root := newBrowser(t, srv.URL)
	root.do("POST", "/auth/login", map[string]any{"handle": "admin", "password": auth.DevPassword}, nil)
	root.do("POST", "/auth/password", map[string]any{"old": auth.DevPassword, "new": "the admin's own"}, nil)
	return srv.URL, b.cookie(auth.SessionCookie), root, cfg.DBPath
}

func hexBatch() map[string]any {
	return map[string]any{"base": "hex-11", "games": 5, "workers": 1, "players": []map[string]any{{"bot": "random"}, {"bot": "random"}}}
}

type limitBody struct {
	Error string `json:"error"`
	Limit string `json:"limit"`
	Max   int    `json:"max"`
}

func TestBatchLimits_REST(t *testing.T) {
	base, ana, admin := limitedServer(t, config.BatchLimits{Games: 5, QueuedPerPlayer: 1})

	// The limits are listed.
	var list struct {
		Limits struct {
			Games          int `json:"games"`
			Workers        int `json:"workers"`
			MCTSIterations int `json:"mcts_iterations"`
			Minutes        int `json:"minutes"`
		} `json:"limits"`
		DefaultWorkers int `json:"default_workers"`
	}
	if code := doJSONMember(t, ana, "GET", base+"/api/batches", nil, &list); code != 200 || list.Limits.Games != 5 || list.Limits.Minutes != 720 || list.Limits.MCTSIterations != 5000 || list.DefaultWorkers < 1 {
		t.Fatalf("the list: %d %+v", code, list)
	}

	// Over a ceiling: 400, with the limit named and its value.
	over := hexBatch()
	over["games"] = 6
	var refused limitBody
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", over, &refused); code != 400 || refused.Limit != "games" || refused.Max != 5 || !strings.Contains(refused.Error, "limit games") {
		t.Errorf("over the games ceiling: %d %+v", code, refused)
	}
	strong := hexBatch()
	strong["players"] = []map[string]any{{"bot": "mcts", "mcts_iterations": 5001}, {"bot": "random"}}
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", strong, &refused); code != 400 || refused.Limit != "mcts_iterations" || refused.Max != 5000 {
		t.Errorf("over the search ceiling: %d %+v", code, refused)
	}

	// An account may not send limits of the batch's own; an administrator's
	// own session may.
	raised := hexBatch()
	raised["games"] = 6
	raised["limits"] = map[string]any{"games": 6}
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", raised, &refused); code != 403 {
		t.Errorf("an account raising its limits: %d %+v, want 403", code, refused)
	}
	var made struct {
		ID    int64 `json:"id"`
		Games int   `json:"games"`
	}
	if code := admin.do("POST", "/api/batches", raised, &made); code != 201 || made.Games != 6 {
		t.Fatalf("an administrator raising a limit: %d %+v", code, made)
	}
	waitBatch(t, base, made.ID, "done")

	// A player with a batch playing has no room for another: 429.
	slow := hexBatch()
	slow["games"] = 5
	slow["players"] = []map[string]any{{"bot": "mcts"}, {"bot": "mcts"}}
	var first struct {
		ID int64 `json:"id"`
	}
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", slow, &first); code != 201 {
		t.Fatalf("a first batch: %d", code)
	}
	t.Cleanup(func() {
		doJSONMember(t, ana, "POST", fmt.Sprintf("%s/api/batches/%d/cancel", base, first.ID), nil, nil)
	})
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", hexBatch(), &refused); code != 429 || refused.Limit != "queued_per_player" || refused.Max != 1 {
		t.Errorf("a second batch while one plays: %d %+v, want 429", code, refused)
	}
}

func waitBatch(t *testing.T, base string, id int64, status string) {
	t.Helper()
	var info struct {
		Status string `json:"status"`
	}
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		doJSON(t, "GET", fmt.Sprintf("%s/api/batches/%d", base, id), nil, &info)
		if info.Status == status {
			return
		}
	}
	t.Fatalf("batch %d is %q, not %q", id, info.Status, status)
}

// MCP's start_batch refuses and reports as the REST route does (for an
// account: a guest or a token has no run_batch, see mcp_caps_test.go).
func TestBatchLimits_MCP(t *testing.T) {
	base, ana, _ := limitedServer(t, config.BatchLimits{Games: 5, QueuedPerPlayer: 1})
	connect := func(headers map[string]string) *client.Client { return mcpWith(t, base, headers) }
	member := map[string]string{"Cookie": auth.SessionCookie + "=" + ana, auth.PageHeader: "1"}
	refusal := func(c *client.Client, spec map[string]any) (int, limitBody) {
		t.Helper()
		res := callTool(t, c, "start_batch", map[string]any{"spec": spec})
		if !res.IsError {
			t.Fatalf("start_batch was not refused: %s", toolErrorText(res))
		}
		var out struct {
			limitBody
			Status int `json:"status"`
		}
		decodeToolResult(t, res, &out)
		return out.Status, out.limitBody
	}
	// An account sends batches (an API token has no scope for them).
	for who, c := range map[string]*client.Client{"an account": connect(member)} {
		over := hexBatch()
		over["games"] = 6
		if status, body := refusal(c, over); status != 400 || body.Limit != "games" || body.Max != 5 {
			t.Errorf("%s over the games ceiling: %d %+v", who, status, body)
		}
		raised := hexBatch()
		raised["games"] = 6
		raised["limits"] = map[string]any{"games": 6}
		if status, _ := refusal(c, raised); status != 403 {
			t.Errorf("%s raising a limit: %d, want 403", who, status)
		}
	}
	// A player who has a batch playing has no room for another.
	c := connect(member)
	slow := hexBatch()
	slow["players"] = []map[string]any{{"bot": "mcts"}, {"bot": "mcts"}}
	res := callTool(t, c, "start_batch", map[string]any{"spec": slow})
	if res.IsError {
		t.Fatalf("a first batch: %s", toolErrorText(res))
	}
	var first struct {
		ID int64 `json:"id"`
	}
	decodeToolResult(t, res, &first)
	t.Cleanup(func() { callTool(t, c, "cancel_batch", map[string]any{"batch_id": first.ID}) })
	if status, body := refusal(c, hexBatch()); status != 429 || body.Limit != "queued_per_player" || body.Max != 1 {
		t.Errorf("a second batch while one plays: %d %+v, want 429", status, body)
	}
}

// A batch started over REST keeps its name, and the name's ceiling bites there.
func TestBatchName_REST(t *testing.T) {
	base, ana, _ := limitedServer(t, config.BatchLimits{})
	body := hexBatch()
	body["name"] = "Sunday ladder"
	var made struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", body, &made); code != 201 || made.Name != "Sunday ladder" {
		t.Fatalf("start: %d %+v", code, made)
	}
	waitBatch(t, base, made.ID, "done")
	var got struct {
		Name string `json:"name"`
	}
	if code := doJSONMember(t, ana, "GET", fmt.Sprintf("%s/api/batches/%d", base, made.ID), nil, &got); code != 200 || got.Name != "Sunday ladder" {
		t.Errorf("get: %d %+v", code, got)
	}
	var list struct {
		Batches []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"batches"`
	}
	doJSONMember(t, ana, "GET", base+"/api/batches", nil, &list)
	found := false
	for _, b := range list.Batches {
		if b.ID == made.ID {
			found = b.Name == "Sunday ladder"
		}
	}
	if !found {
		t.Errorf("the list lost the name: %+v", list.Batches)
	}
	body["name"] = strings.Repeat("x", 201)
	var refused limitBody
	if code := doJSONMember(t, ana, "POST", base+"/api/batches", body, &refused); code != 400 || refused.Limit != "name" || refused.Max != 200 {
		t.Errorf("a long name: %d %+v", code, refused)
	}
}
