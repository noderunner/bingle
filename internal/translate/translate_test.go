package translate

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/config"
)

// fake is a translation service for the tests: answer decides what it says to
// each request, and it remembers what it was sent.
type fake struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    []chatRequest
	calls  atomic.Int32
	answer func(req chatRequest) (status int, content string)
}

func newFake(t *testing.T, answer func(req chatRequest) (int, string)) *fake {
	t.Helper()
	f := &fake{answer: answer}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.calls.Add(1)
		f.mu.Lock()
		f.got = append(f.got, req)
		f.mu.Unlock()
		status, content := f.answer(req)
		if status != http.StatusOK {
			http.Error(w, "no", status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}},
		})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) last() chatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got[len(f.got)-1]
}

// reply is a model's answer.
func reply(source, text string) string {
	b, _ := json.Marshal(answer{Source: source, Text: text})
	return string(b)
}

func newService(t *testing.T, f *fake, workers int, timeout time.Duration) *Service {
	t.Helper()
	s, err := New(config.Translate{URL: f.srv.URL + "/v1", Model: "test-model", API: "openai", Workers: workers, Timeout: timeout}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestTranslate_ALineReachesTheReaderInTheirLanguage(t *testing.T) {
	f := newFake(t, func(req chatRequest) (int, string) {
		// The model sees the markers, never the handle, the link or the move.
		return http.StatusOK, reply("es", "⟦1⟧, play ⟦2⟧ and look at ⟦3⟧ ⟦4⟧")
	})
	s := newService(t, f, 1, 5*time.Second)
	res, err := s.Translate(context.Background(), Request{
		Text: "@ana, juega a1-b2 y mira https://example.com/x 👍", Target: "en", Hint: "es",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Outcome: Translated, Source: "es", Text: "@ana, play a1-b2 and look at https://example.com/x 👍"}
	if res != want {
		t.Fatalf("got %+v, want %+v", res, want)
	}
	req := f.last()
	if req.Model != "test-model" || len(req.Messages) != 2 {
		t.Fatalf("request: %+v", req)
	}
	sys, user := req.Messages[0], req.Messages[1]
	if sys.Role != "system" || user.Role != "user" {
		t.Fatalf("roles: %q, %q", sys.Role, user.Role)
	}
	if user.Content != "⟦1⟧, juega ⟦2⟧ y mira ⟦3⟧ ⟦4⟧" {
		t.Fatalf("the line sent was %q", user.Content)
	}
	if strings.Contains(sys.Content, "juega") || !strings.Contains(sys.Content, "into English") || !strings.Contains(sys.Content, "in Spanish") {
		t.Fatalf("the instructions are not fixed: %q", sys.Content)
	}
	if req.ResponseFormat["type"] != "json_schema" {
		t.Fatalf("response_format: %v", req.ResponseFormat)
	}
}

func TestTranslate_NamesInTheRoomAreKept(t *testing.T) {
	f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, reply("fr", "well played ⟦1⟧") })
	s := newService(t, f, 1, 5*time.Second)
	res, err := s.Translate(context.Background(), Request{Text: "bien joué Rose", Target: "en", Keep: []string{"Rose", "Ros"}})
	if err != nil || res.Text != "well played Rose" {
		t.Fatalf("got %+v, %v", res, err)
	}
	if got := f.last().Messages[1].Content; got != "bien joué ⟦1⟧" {
		t.Fatalf("sent %q", got)
	}
}

func TestTranslate_ALineWithNoWordsIsNotSent(t *testing.T) {
	f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, reply("en", "x") })
	s := newService(t, f, 1, 5*time.Second)
	for _, line := range []string{"c3", "a1-b2 e2e4 Nf3 exd5 O-O-O", "@bob 👍👍", "https://example.com", "⟦1⟧ hola"} {
		res, err := s.Translate(context.Background(), Request{Text: line, Target: "en"})
		if err != nil || res.Outcome != Skipped {
			t.Errorf("%q: got %+v, %v", line, res, err)
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Fatalf("the service was asked %d times", n)
	}
}

func TestTranslate_ALineInTheReadersLanguageIsTheSame(t *testing.T) {
	for _, source := range []string{"pt-BR", "pt", "pt_PT"} {
		f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, reply(source, "boa jogada") })
		s := newService(t, f, 1, 5*time.Second)
		res, err := s.Translate(context.Background(), Request{Text: "boa jogada", Target: "pt-BR"})
		if err != nil || res.Outcome != Same || res.Source != "pt" {
			t.Errorf("%s: got %+v, %v", source, res, err)
		}
	}
}

func TestTranslate_AnAnswerThatIsNotATranslationIsDropped(t *testing.T) {
	cases := map[string]string{
		"a marker lost":      reply("es", "play and look"),
		"a marker repeated":  reply("es", "play ⟦1⟧ ⟦1⟧"),
		"a marker invented":  reply("es", "play ⟦1⟧ ⟦2⟧"),
		"a stray bracket":    reply("es", "play ⟦1⟧ ⟦"),
		"a line break":       reply("es", "play ⟦1⟧\nSure! Here is the translation."),
		"markup":             reply("es", "play <b>⟦1⟧</b>"),
		"a link in markdown": reply("es", "[play](http://x) ⟦1⟧"),
		"far too long":       reply("es", "play ⟦1⟧ "+strings.Repeat("and then some more words ", 10)),
		"empty":              reply("es", " "),
		"not JSON":           "I'm sorry, I can't help with that.",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, content })
			s := newService(t, f, 1, 5*time.Second)
			_, err := s.Translate(context.Background(), Request{Text: "juega ahora mismo a1-b2", Target: "en"})
			if !errors.Is(err, ErrRejected) {
				t.Fatalf("got %v, want ErrRejected", err)
			}
		})
	}
}

func TestTranslate_ThinkingAndAFenceAroundTheAnswerAreTakenOff(t *testing.T) {
	f := newFake(t, func(chatRequest) (int, string) {
		return http.StatusOK, "<think>It is Spanish.</think>\n```json\n" + reply("es", "good game") + "\n```"
	})
	s := newService(t, f, 1, 5*time.Second)
	res, err := s.Translate(context.Background(), Request{Text: "buena partida", Target: "en"})
	if err != nil || res.Text != "good game" {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestTranslate_ADownServiceIsPausedAndTriedAgainLater(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	f := newFake(t, func(chatRequest) (int, string) {
		if down.Load() {
			return http.StatusServiceUnavailable, ""
		}
		return http.StatusOK, reply("es", "hello")
	})
	s := newService(t, f, 1, 5*time.Second)
	now := time.Now()
	s.now = func() time.Time { return now }
	req := Request{Text: "hola", Target: "en"}
	for i := range failuresToPause {
		if _, err := s.Translate(context.Background(), req); err == nil || errors.Is(err, ErrPaused) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := s.Translate(context.Background(), req); !errors.Is(err, ErrPaused) {
		t.Fatalf("got %v, want ErrPaused", err)
	}
	if s.Submit(req, func(Result, error) { t.Error("called while paused") }) {
		t.Fatal("queued while paused")
	}
	if n := f.calls.Load(); n != failuresToPause {
		t.Fatalf("asked %d times while paused", n)
	}
	down.Store(false)
	now = now.Add(pauseFor)
	if res, err := s.Translate(context.Background(), req); err != nil || res.Text != "hello" {
		t.Fatalf("after the pause: %+v, %v", res, err)
	}
}

func TestTranslate_ASlowServiceTimesOutAndSubmitDoesNotWait(t *testing.T) {
	release := make(chan struct{})
	f := newFake(t, func(chatRequest) (int, string) {
		<-release
		return http.StatusOK, reply("es", "hello")
	})
	defer close(release)
	s := newService(t, f, 1, time.Second)
	got := make(chan error, 1)
	start := time.Now()
	if !s.Submit(Request{Text: "hola", Target: "en"}, func(_ Result, err error) { got <- err }) {
		t.Fatal("not queued")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("Submit waited %v", d)
	}
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("a slow service answered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the timeout did not end the request")
	}
}

func TestTranslate_AFullQueueDropsRequests(t *testing.T) {
	release := make(chan struct{})
	f := newFake(t, func(chatRequest) (int, string) {
		<-release
		return http.StatusOK, reply("es", "hello")
	})
	s := newService(t, f, 1, 30*time.Second)
	var done atomic.Int32
	queued, dropped := 0, 0
	for range queueSize + 10 {
		if s.Submit(Request{Text: "hola", Target: "en"}, func(Result, error) { done.Add(1) }) {
			queued++
		} else {
			dropped++
		}
	}
	// One is with the worker (or about to be), queueSize wait.
	if dropped < 9 || queued > queueSize+1 {
		t.Fatalf("queued %d, dropped %d", queued, dropped)
	}
	close(release)
	s.Close()
	if int(done.Load()) != queued {
		t.Fatalf("%d of %d queued requests were answered", done.Load(), queued)
	}
}

func TestTranslate_ATargetOutsideTheInterfaceIsRefused(t *testing.T) {
	f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, reply("es", "x") })
	s := newService(t, f, 1, time.Second)
	if _, err := s.Translate(context.Background(), Request{Text: "hola", Target: "it"}); err == nil {
		t.Fatal("translated into a language the interface does not have")
	}
}

func TestProtect_WhatIsNotLanguageIsKept(t *testing.T) {
	cases := map[string]string{
		"take c3 now":                   "take ⟦1⟧ now",
		"a1-b2, then e2e4!":             "⟦1⟧, then ⟦2⟧!",
		"Nf3+ is fine, exd5 too":        "⟦1⟧+ is fine, ⟦2⟧ too",
		"castle O-O-O.":                 "castle ⟦1⟧.",
		"ask @rose.b_2 here":            "ask ⟦1⟧ here",
		"see (https://x.org/a?b=1).":    "see (⟦1⟧).",
		"gg 👍🏽 and ❤️":                  "gg ⟦1⟧ and ⟦2⟧",
		"nothing to keep at all":        "nothing to keep at all",
		"l'été à Paris":                 "l'été à Paris",
		"كش ملك c3":                     "كش ملك ⟦1⟧",
		"王手 a1-b2 です":                   "王手 ⟦1⟧ です",
		"mail me at a@b.example please": "mail me at a⟦1⟧ please",
	}
	for in, want := range cases {
		p, ok := protect(in, nil)
		if !ok || p.text != want {
			t.Errorf("protect(%q) = %q, want %q", in, p.text, want)
			continue
		}
		back, ok := restore(p.text, p)
		if !ok || back != in {
			t.Errorf("restore(%q) = %q, want %q", p.text, back, in)
		}
	}
}

func TestProtect_KeepMatchesWholeWordsOnly(t *testing.T) {
	p, _ := protect("Ann and Annette", []string{"Ann"})
	if p.text != "⟦1⟧ and Annette" {
		t.Fatalf("got %q", p.text)
	}
}

func TestLocal_TheServiceShouldBeOnThisMachineOrNetwork(t *testing.T) {
	lookup := func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "laptop.lan":
			return []net.IP{net.ParseIP("192.168.1.20")}, nil
		case "mixed.example":
			return []net.IP{net.ParseIP("10.0.0.2"), net.ParseIP("203.0.113.9")}, nil
		}
		return nil, errors.New("no such host")
	}
	cases := map[string]bool{
		"http://127.0.0.1:11434/v1":    true,
		"http://localhost:11434/v1":    true,
		"http://[::1]:8080/v1":         true,
		"http://192.168.1.20:11434/v1": true,
		"http://10.1.2.3/v1":           true,
		"http://laptop.lan:11434/v1":   true,
		"http://203.0.113.9/v1":        false,
		"https://mixed.example/v1":     false,
	}
	for u, want := range cases {
		got, err := Local(context.Background(), u, lookup)
		if err != nil || got != want {
			t.Errorf("Local(%s) = %v, %v; want %v", u, got, err, want)
		}
	}
	if _, err := Local(context.Background(), "http://nowhere.example/v1", lookup); err == nil {
		t.Error("an unknown host was not an error")
	}
}

func TestTranslate_AThinkingModelIsAskedToAnswerAtOnce(t *testing.T) {
	f := newFake(t, func(chatRequest) (int, string) { return http.StatusOK, reply("es", "hello") })
	s := newService(t, f, 1, 5*time.Second)
	if _, err := s.Translate(context.Background(), Request{Text: "hola", Target: "en"}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().ReasoningEffort; got != "none" {
		t.Fatalf("reasoning_effort = %q, want none", got)
	}
}

func TestTranslate_AServerThatRefusesTheThinkingFieldIsAskedWithoutIt(t *testing.T) {
	f := newFake(t, func(req chatRequest) (int, string) {
		if req.ReasoningEffort != "" {
			return http.StatusBadRequest, ""
		}
		return http.StatusOK, reply("es", "hello")
	})
	s := newService(t, f, 1, 5*time.Second)
	for range 2 {
		res, err := s.Translate(context.Background(), Request{Text: "hola", Target: "en"})
		if err != nil || res.Text != "hello" {
			t.Fatalf("got %+v, %v", res, err)
		}
	}
	// Refused once, then never sent again: three requests for two lines.
	if n := f.calls.Load(); n != 3 {
		t.Fatalf("asked %d times, want 3", n)
	}
}
