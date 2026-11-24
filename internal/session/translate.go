package session

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/translate"
)

// Chat in each reader's language (ADR 0143).
//
// A reader whose socket is open in a room registers their language there; a
// line said in the room goes to the translation service once for each language
// registered, and what it comes to is kept with the line and announced in a
// "chat_translation" event, which each socket passes on only to a reader of
// that language. Nothing waits for the service: the "chat" event has gone
// already, and a line the service does not answer stays as written.

// Reader is who reads a request's chat: the language the interface speaks to
// them in, and whether they want chat shown in it.
type Reader struct {
	Lang      string
	Translate bool
}

type readerKey struct{}

// WithReader carries the request's reader, for the chat it reads and the hint
// of what language the lines it writes are in.
func WithReader(ctx context.Context, r Reader) context.Context {
	return context.WithValue(ctx, readerKey{}, r)
}

func readerFrom(ctx context.Context) (Reader, bool) {
	r, ok := ctx.Value(readerKey{}).(Reader)
	return r, ok
}

// ChatTranslation is a line in a reader's language.
type ChatTranslation struct {
	// Lang is the reader's language, From the line's as the service saw it
	// (a BCP 47 primary subtag, "und" when it gave none).
	Lang string `json:"lang"`
	From string `json:"from"`
	Text string `json:"text"`
}

// ChatTranslationEventData is a "chat_translation" event: line ID in one
// reader's language. A socket sends only its reader's.
type ChatTranslationEventData struct {
	ID int64 `json:"id"`
	ChatTranslation
}

// historyBackfill is how many of the latest lines without a translation a
// reader's history asks for: those said before a reader of that language came.
const historyBackfill = 20

// chatTranslation is the manager's translation state.
type chatTranslation struct {
	svc *translate.Service
	// mayRead says whether a request's reader may have chat translated, and
	// allowLine takes one of a writer's lines from their limit.
	mayRead   func(context.Context) bool
	allowLine func(context.Context) bool

	mu sync.Mutex
	// readers counts the readers of each language in each room.
	readers map[string]map[string]int
	// pending are the lines asked for and not yet answered, by
	// pendingKey: one request for a line and language at a time.
	pending map[string]bool
}

// EnableTranslate turns chat translation on, through svc. mayRead and
// allowLine are the capability check and the writer's limit (nil allows).
// Call it before the server starts.
func (m *Manager) EnableTranslate(svc *translate.Service, mayRead, allowLine func(context.Context) bool) {
	if svc == nil {
		return
	}
	if mayRead == nil {
		mayRead = func(context.Context) bool { return true }
	}
	if allowLine == nil {
		allowLine = func(context.Context) bool { return true }
	}
	m.translation = &chatTranslation{svc: svc, mayRead: mayRead, allowLine: allowLine,
		readers: map[string]map[string]int{}, pending: map[string]bool{}}
}

// TranslationLanguage is the language the request's reader has chat shown in,
// or "" when they have it as written: translation is off here, they turned it
// off, may not have it, or read in a language the service is not asked for.
func (m *Manager) TranslationLanguage(ctx context.Context) string {
	t := m.translation
	if t == nil {
		return ""
	}
	r, ok := readerFrom(ctx)
	if !ok || !r.Translate || !translate.Supported(r.Lang) || !t.mayRead(ctx) {
		return ""
	}
	return r.Lang
}

// ReadIn registers the request's reader in room while their socket is open,
// so the lines said there are put into their language; the function returned
// ends that. A reader who has chat as written registers nothing.
func (m *Manager) ReadIn(ctx context.Context, room string) func() {
	lang := m.TranslationLanguage(ctx)
	if lang == "" {
		return func() {}
	}
	t := m.translation
	t.mu.Lock()
	if t.readers[room] == nil {
		t.readers[room] = map[string]int{}
	}
	t.readers[room][lang]++
	t.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.readers[room][lang]--
			if t.readers[room][lang] <= 0 {
				delete(t.readers[room], lang)
			}
			if len(t.readers[room]) == 0 {
				delete(t.readers, room)
			}
		})
	}
}

// languagesIn are the languages of the readers in room.
func (t *chatTranslation) languagesIn(room string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.readers[room]))
	for lang := range t.readers[room] {
		out = append(out, lang)
	}
	return out
}

// lineToTranslate is a line said in a room, as the service needs it.
type lineToTranslate struct {
	global bool
	room   string
	id     int64
	text   string
	// keep are the names in the room, never translated.
	keep []string
	// hint is the writer's language, "" when unknown.
	hint string
}

func pendingKey(global bool, id int64, lang string) string {
	kind := "g"
	if global {
		kind = "w"
	}
	return kind + strconv.FormatInt(id, 10) + "|" + lang
}

// translateSaid asks for a line just said in each language read in its room.
// The writer's limit is taken once for the line, whatever the languages.
func (m *Manager) translateSaid(ctx context.Context, l lineToTranslate) {
	t := m.translation
	if t == nil {
		return
	}
	langs := t.languagesIn(l.room)
	if len(langs) == 0 || !t.allowLine(ctx) {
		return
	}
	if r, ok := readerFrom(ctx); ok {
		l.hint = r.Lang
	}
	for _, lang := range langs {
		m.askTranslation(l, lang)
	}
}

// askTranslation queues one line for one language. What it comes to is kept,
// so it is never asked for again, and a translation is announced in the room.
// A failure keeps nothing: the line is asked for again when a history wants it.
func (m *Manager) askTranslation(l lineToTranslate, lang string) {
	t := m.translation
	key := pendingKey(l.global, l.id, lang)
	t.mu.Lock()
	if t.pending[key] {
		t.mu.Unlock()
		return
	}
	t.pending[key] = true
	t.mu.Unlock()
	done := func() {
		t.mu.Lock()
		delete(t.pending, key)
		t.mu.Unlock()
	}
	queued := t.svc.Submit(translate.Request{Text: l.text, Target: lang, Hint: l.hint, Keep: l.keep}, func(res translate.Result, err error) {
		defer done()
		if err != nil && !errors.Is(err, translate.ErrRejected) {
			return
		}
		kept := store.ChatTranslation{Source: res.Source}
		if res.Outcome == translate.Translated {
			kept.Body = res.Text
		}
		ctx := context.Background()
		if err := m.store.AddChatTranslation(ctx, l.global, l.id, lang, kept); err != nil {
			m.logger().Warn("translate: a translation was not kept", "err", err)
		}
		if kept.Body == "" {
			return
		}
		m.hub.publish(l.room, Event{Type: "chat_translation", Data: ChatTranslationEventData{
			ID: l.id, ChatTranslation: ChatTranslation{Lang: lang, From: res.Source, Text: res.Text}}})
	})
	if !queued {
		done()
	}
}

// translationsOf gives each line its kept translation in the request's
// reader's language, and asks for the latest of those that have none yet.
// line says, for an index, the line's id, text and whether the server said it,
// and set gives it its translation.
func (m *Manager) translationsOf(ctx context.Context, global bool, room string, n int, keep []string,
	line func(i int) (id int64, text string, server bool), set func(i int, t ChatTranslation)) {
	lang := m.TranslationLanguage(ctx)
	if lang == "" || n == 0 {
		return
	}
	ids := make([]int64, 0, n)
	for i := range n {
		if id, _, server := line(i); !server {
			ids = append(ids, id)
		}
	}
	kept, err := m.store.ChatTranslations(ctx, global, ids, lang)
	if err != nil {
		m.logger().Warn("translate: reading translations", "err", err)
		return
	}
	asked := 0
	for i := n - 1; i >= 0; i-- {
		id, text, server := line(i)
		if server {
			continue
		}
		k, ok := kept[id]
		switch {
		case ok && k.Body != "":
			set(i, ChatTranslation{Lang: lang, From: k.Source, Text: k.Body})
		case !ok && asked < historyBackfill:
			asked++
			m.askTranslation(lineToTranslate{global: global, room: room, id: id, text: text, keep: keep}, lang)
		}
	}
}
