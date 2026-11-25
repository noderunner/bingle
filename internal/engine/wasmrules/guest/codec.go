package guest

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// The JSON of a call, read and written by hand. encoding/json works by
// reflection, which is the slowest thing a Go module does: compiled to
// WebAssembly it took nearly all of a call's time (ADR 0147's status log).
// What is on the wire is the same JSON either way. Only init, which an
// instance hears once for a game, is still read with encoding/json.

// decoder reads one JSON value from data. The first error sticks: after it
// every read returns nothing and every loop ends.
type decoder struct {
	data []byte
	pos  int
	err  error
	// fresh is true just after an object or an array was opened, where no
	// comma comes before the next member.
	fresh bool
	depth int
	// seen are the few short strings a position repeats (its pieces' types
	// and colours), so each is made once for a call.
	seen  [8]string
	nseen int
	// stacks is the memory the stacks of a position's cells are cut from.
	stacks []Piece
}

// maxDepth is how deep a value may nest.
const maxDepth = 200

func (d *decoder) fail(what string) {
	if d.err == nil {
		d.err = fmt.Errorf("%s at byte %d", what, d.pos)
	}
	d.pos = len(d.data)
}

// peek is the next byte that is not white space, 0 at the end.
func (d *decoder) peek() byte {
	for d.pos < len(d.data) {
		switch c := d.data[d.pos]; c {
		case ' ', '\t', '\n', '\r':
			d.pos++
		default:
			return c
		}
	}
	return 0
}

func (d *decoder) literal(word string) {
	if len(d.data)-d.pos < len(word) || string(d.data[d.pos:d.pos+len(word)]) != word {
		d.fail("not a JSON value")
		return
	}
	d.pos += len(word)
}

// null reads a null if one is next. A null leaves a field as it was.
func (d *decoder) null() bool {
	if d.err != nil || d.peek() != 'n' {
		return false
	}
	d.literal("null")
	return d.err == nil
}

// open enters an object ('{') or an array ('['), and reports whether there
// is one to read: a null is none.
func (d *decoder) open(bracket byte) bool {
	if d.err != nil || d.null() {
		return false
	}
	if d.peek() != bracket {
		d.fail("expected " + string(bracket))
		return false
	}
	if d.depth++; d.depth > maxDepth {
		d.fail("nested too deeply")
		return false
	}
	d.pos++
	d.fresh = true
	return true
}

// next moves to the next member of the object or array that open entered,
// and reports whether there is one. It reads the bracket that closes it.
func (d *decoder) next(closing byte) bool {
	if d.err != nil {
		return false
	}
	c := d.peek()
	if c == closing {
		d.pos++
		d.depth--
		d.fresh = false
		return false
	}
	if d.fresh {
		d.fresh = false
		return true
	}
	if c != ',' {
		d.fail("expected , or " + string(closing))
		return false
	}
	d.pos++
	return true
}

// key reads a member's name and its colon. The bytes are the input's own
// unless the name has an escape in it.
func (d *decoder) key() []byte {
	k := d.text()
	if d.err == nil && d.peek() != ':' {
		d.fail("expected :")
	}
	if d.err != nil {
		return nil
	}
	d.pos++
	return k
}

// text reads a string and returns what it says, as the input's own bytes
// where it has no escape.
func (d *decoder) text() []byte {
	if d.err != nil {
		return nil
	}
	if d.peek() != '"' {
		d.fail("expected a string")
		return nil
	}
	d.pos++
	start := d.pos
	for i := start; i < len(d.data); i++ {
		switch c := d.data[i]; {
		case c == '"':
			d.pos = i + 1
			return d.data[start:i]
		case c == '\\':
			return d.escaped(start, i)
		case c < 0x20:
			d.pos = i
			d.fail("a control character in a string")
			return nil
		}
	}
	d.fail("a string without an end")
	return nil
}

// escaped reads the rest of a string whose first escape is at i.
func (d *decoder) escaped(start, i int) []byte {
	out := append(make([]byte, 0, len(d.data)-start), d.data[start:i]...)
	for i < len(d.data) {
		c := d.data[i]
		switch {
		case c == '"':
			d.pos = i + 1
			return out
		case c < 0x20:
			d.pos = i
			d.fail("a control character in a string")
			return nil
		case c != '\\':
			out = append(out, c)
			i++
			continue
		}
		if i++; i >= len(d.data) {
			break
		}
		switch c = d.data[i]; c {
		case '"', '\\', '/':
			out = append(out, c)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, ok := hex4(d.data, i+1)
			if !ok {
				d.pos = i
				d.fail("a bad \\u escape")
				return nil
			}
			i += 4
			if utf16.IsSurrogate(r) {
				// A pair of escapes for one character, or else nothing valid.
				low, ok := rune(0), false
				if i+2 < len(d.data) && d.data[i+1] == '\\' && d.data[i+2] == 'u' {
					low, ok = hex4(d.data, i+3)
				}
				if pair := utf16.DecodeRune(r, low); ok && pair != utf8.RuneError {
					r = pair
					i += 6
				} else {
					r = utf8.RuneError
				}
			}
			out = utf8.AppendRune(out, r)
		default:
			d.pos = i
			d.fail("a bad escape in a string")
			return nil
		}
		i++
	}
	d.fail("a string without an end")
	return nil
}

func hex4(data []byte, at int) (rune, bool) {
	if at+4 > len(data) {
		return 0, false
	}
	var r rune
	for _, c := range data[at : at+4] {
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

// str reads a string, or a null as none.
func (d *decoder) str() string {
	if d.null() {
		return ""
	}
	return string(d.text())
}

// short reads a string that a position says over and over, and makes it once.
func (d *decoder) short() string {
	if d.null() {
		return ""
	}
	b := d.text()
	for _, s := range d.seen[:d.nseen] {
		if s == string(b) {
			return s
		}
	}
	s := string(b)
	if d.nseen < len(d.seen) && d.err == nil {
		d.seen[d.nseen] = s
		d.nseen++
	}
	return s
}

// whole reads a whole number that is at most max, and whether it had a
// minus sign.
func (d *decoder) whole(max uint64) (n uint64, negative bool) {
	if d.err != nil || d.null() {
		return 0, false
	}
	if d.peek() == '-' {
		negative = true
		d.pos++
	}
	start := d.pos
	for d.pos < len(d.data) {
		c := d.data[d.pos]
		if c < '0' || c > '9' {
			break
		}
		digit := uint64(c - '0')
		if n > (max-digit)/10 {
			d.fail("a number too large")
			return 0, false
		}
		n = n*10 + digit
		d.pos++
	}
	switch {
	case d.pos == start:
		d.fail("expected a number")
	case d.pos-start > 1 && d.data[start] == '0':
		d.pos = start
		d.fail("a number with a leading zero")
	case d.pos < len(d.data) && (d.data[d.pos] == '.' || d.data[d.pos] == 'e' || d.data[d.pos] == 'E'):
		d.fail("expected a whole number")
	}
	if d.err != nil {
		return 0, false
	}
	return n, negative
}

func (d *decoder) int() int {
	n, negative := d.whole(math.MaxInt)
	if negative {
		return -int(n)
	}
	return int(n)
}

func (d *decoder) uint64() uint64 {
	n, negative := d.whole(math.MaxUint64)
	if negative && n != 0 {
		d.fail("a number below zero")
		return 0
	}
	return n
}

// skip reads any value and keeps nothing of it.
func (d *decoder) skip() {
	if d.err != nil {
		return
	}
	switch c := d.peek(); {
	case c == '"':
		d.text()
	case c == '{':
		if d.open('{') {
			for d.next('}') {
				d.key()
				d.skip()
			}
		}
	case c == '[':
		if d.open('[') {
			for d.next(']') {
				d.skip()
			}
		}
	case c == 't':
		d.literal("true")
	case c == 'f':
		d.literal("false")
	case c == 'n':
		d.literal("null")
	case c == '-' || (c >= '0' && c <= '9'):
		d.number()
	default:
		d.fail("not a JSON value")
	}
}

// number reads any JSON number.
func (d *decoder) number() {
	digits := func() bool {
		start := d.pos
		for d.pos < len(d.data) && d.data[d.pos] >= '0' && d.data[d.pos] <= '9' {
			d.pos++
		}
		return d.pos > start
	}
	at := func(set string) bool {
		if d.pos < len(d.data) {
			for i := 0; i < len(set); i++ {
				if d.data[d.pos] == set[i] {
					return true
				}
			}
		}
		return false
	}
	if at("-") {
		d.pos++
	}
	start := d.pos
	ok := digits() && (d.pos-start == 1 || d.data[start] != '0')
	if ok && at(".") {
		d.pos++
		ok = digits()
	}
	if ok && at("eE") {
		d.pos++
		if at("+-") {
			d.pos++
		}
		ok = digits()
	}
	if !ok {
		d.fail("not a JSON number")
	}
}

// raw reads any value and returns a copy of its JSON.
func (d *decoder) raw() []byte {
	d.peek()
	start := d.pos
	d.skip()
	if d.err != nil {
		return nil
	}
	return append([]byte(nil), d.data[start:d.pos]...)
}

// end fails unless nothing but white space is left.
func (d *decoder) end() {
	if d.err == nil && d.peek() != 0 {
		d.fail("something after the value")
	}
}

// valid reports whether data is one JSON value.
func valid(data []byte) bool {
	d := decoder{data: data}
	d.skip()
	d.end()
	return d.err == nil
}

// request is what a call other than init is asked with: a call reads the
// fields it takes.
type request struct {
	Game string
	// Position is the position, and Version its document's "v".
	Position  *Position
	Version   int
	Seat      int
	Selection *Selection
	Text      string
	Seed      uint64
	// Bot is the bot call's request, read when it is answered.
	Bot []byte
}

func decodeRequest(data []byte) (request, error) {
	var r request
	d := decoder{data: data}
	if !d.open('{') && d.err == nil {
		d.fail("expected an object")
	}
	for d.next('}') {
		switch string(d.key()) {
		case "game":
			r.Game = d.str()
		case "position":
			r.Position, r.Version = d.position()
		case "seat":
			r.Seat = d.int()
		case "selection":
			r.Selection = d.selection()
		case "text":
			r.Text = d.str()
		case "seed":
			r.Seed = d.uint64()
		case "bot":
			r.Bot = d.raw()
		default:
			d.skip()
		}
	}
	d.end()
	return r, d.err
}

// position reads a position document and its version. Fields of the
// engine's document that a ruleset's position never has are passed over.
func (d *decoder) position() (*Position, int) {
	if !d.open('{') {
		return nil, 0
	}
	p, version := &Position{}, 0
	for d.next('}') {
		switch string(d.key()) {
		case "v":
			version = d.int()
		case "cells":
			if d.open('{') {
				p.Cells = make(map[string][]Piece, 32)
				for d.next('}') {
					id := string(d.key())
					p.Cells[id] = d.stack()
				}
			}
		case "reserves":
			if d.open('{') {
				p.Reserves = map[string]map[string]int{}
				for d.next('}') {
					color := string(d.key())
					p.Reserves[color] = d.counts()
				}
			}
		case "ply":
			p.Ply = d.int()
		case "counters":
			p.Counters = d.counts()
		case "ext":
			if !d.null() {
				p.Ext = d.raw()
			}
		default:
			d.skip()
		}
	}
	return p, version
}

// stack reads the pieces on one cell, bottom first. A stack is cut to its
// length, so adding to one never writes into the next.
func (d *decoder) stack() []Piece {
	if !d.open('[') {
		return nil
	}
	if d.stacks == nil {
		d.stacks = make([]Piece, 0, 48)
	}
	start := len(d.stacks)
	for d.next(']') {
		var piece Piece
		if d.open('{') {
			for d.next('}') {
				switch string(d.key()) {
				case "piece":
					piece.Piece = d.short()
				case "color":
					piece.Color = d.short()
				default:
					d.skip()
				}
			}
		}
		d.stacks = append(d.stacks, piece)
	}
	return d.stacks[start:len(d.stacks):len(d.stacks)]
}

func (d *decoder) counts() map[string]int {
	if !d.open('{') {
		return nil
	}
	out := map[string]int{}
	for d.next('}') {
		name := string(d.key())
		out[name] = d.int()
	}
	return out
}

func (d *decoder) selection() *Selection {
	if !d.open('{') {
		return nil
	}
	s := &Selection{}
	for d.next('}') {
		switch string(d.key()) {
		case "action":
			s.Action = d.str()
		case "piece":
			s.Piece = d.str()
		case "from":
			s.From = d.str()
		case "to":
			s.To = d.str()
		case "count":
			s.Count = d.int()
		case "path":
			if d.open('[') {
				s.Path = []string{}
				for d.next(']') {
					s.Path = append(s.Path, d.str())
				}
			}
		case "drops":
			if d.open('[') {
				s.Drops = []int{}
				for d.next(']') {
					s.Drops = append(s.Drops, d.int())
				}
			}
		case "aim":
			s.Aim = d.str()
		case "card":
			s.Card = d.str()
		default:
			d.skip()
		}
	}
	return s
}

// What follows writes replies. A member that is empty is left out where the
// engine's own JSON leaves it out, and the members of a map are written in
// the order of their names, so the same answer is always the same bytes.

const hexDigits = "0123456789abcdef"

func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue
		}
		b = append(b, s[start:i]...)
		switch c {
		case '"', '\\':
			b = append(b, '\\', c)
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		}
		start = i + 1
	}
	return append(append(b, s[start:]...), '"')
}

func appendInt(b []byte, n int) []byte { return strconv.AppendInt(b, int64(n), 10) }

// appendMember writes `,"name":` and the comma only after an earlier member.
func appendMember(b []byte, open int, name string) []byte {
	if len(b) > open {
		b = append(b, ',')
	}
	return append(append(append(b, '"'), name...), '"', ':')
}

func appendStrings(b []byte, list []string) []byte {
	b = append(b, '[')
	for i, s := range list {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendString(b, s)
	}
	return append(b, ']')
}

func appendInts(b []byte, list []int) []byte {
	b = append(b, '[')
	for i, n := range list {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendInt(b, n)
	}
	return append(b, ']')
}

func appendCounts(b []byte, counts map[string]int) []byte {
	b = append(b, '{')
	open := len(b)
	for _, name := range sortedKeys(counts) {
		b = appendInt(appendKey(b, open, name), counts[name])
	}
	return append(b, '}')
}

// appendKey is appendMember for a name that is not one of the wire's own, and
// so may need escaping.
func appendKey(b []byte, open int, name string) []byte {
	if len(b) > open {
		b = append(b, ',')
	}
	return append(appendString(b, name), ':')
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

var errExt = errors.New("guest: a position's ext is not JSON")

func appendPosition(b []byte, p *Position) ([]byte, error) {
	b = append(b, `{"v":`...)
	b = appendInt(b, positionVersion)
	b = append(b, `,"cells":{`...)
	open := len(b)
	for _, id := range sortedKeys(p.Cells) {
		b = append(appendKey(b, open, id), '[')
		for i, piece := range p.Cells[id] {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendString(append(b, `{"piece":`...), piece.Piece)
			b = append(appendString(append(b, `,"color":`...), piece.Color), '}')
		}
		b = append(b, ']')
	}
	b = append(b, '}')
	if len(p.Reserves) > 0 {
		b = append(b, `,"reserves":{`...)
		open := len(b)
		for _, color := range sortedKeys(p.Reserves) {
			b = appendCounts(appendKey(b, open, color), p.Reserves[color])
		}
		b = append(b, '}')
	}
	b = appendInt(append(b, `,"ply":`...), p.Ply)
	if len(p.Counters) > 0 {
		b = appendCounts(append(b, `,"counters":`...), p.Counters)
	}
	if len(p.Ext) > 0 {
		if !valid(p.Ext) {
			return nil, errExt
		}
		b = append(append(b, `,"ext":`...), p.Ext...)
	}
	return append(b, '}'), nil
}

func appendSelection(b []byte, s *Selection) []byte {
	b = append(b, '{')
	open := len(b)
	text := func(name, value string) {
		if value != "" {
			b = appendString(appendMember(b, open, name), value)
		}
	}
	text("action", s.Action)
	text("piece", s.Piece)
	text("from", s.From)
	text("to", s.To)
	if s.Count != 0 {
		b = appendInt(appendMember(b, open, "count"), s.Count)
	}
	if len(s.Path) > 0 {
		b = appendStrings(appendMember(b, open, "path"), s.Path)
	}
	if len(s.Drops) > 0 {
		b = appendInts(appendMember(b, open, "drops"), s.Drops)
	}
	text("aim", s.Aim)
	text("card", s.Card)
	return append(b, '}')
}

// appendOutcome writes the members of an outcome into an object that is
// open, whose first member would be at open.
func appendOutcome(b []byte, open int, o *Outcome) []byte {
	if o.Ended {
		b = append(appendMember(b, open, "ended"), "true"...)
	}
	if o.Winner != 0 {
		b = appendInt(appendMember(b, open, "winner"), o.Winner)
	}
	if len(o.Drawn) > 0 {
		b = appendInts(appendMember(b, open, "drawn"), o.Drawn)
	}
	if o.AllLost {
		b = append(appendMember(b, open, "all_lost"), "true"...)
	}
	if o.Reason != "" {
		b = appendString(appendMember(b, open, "reason"), o.Reason)
	}
	if o.Detail != "" {
		b = appendString(appendMember(b, open, "detail"), o.Detail)
	}
	return b
}

// appendResult writes the reply of apply.
func appendResult(b []byte, res *Result, notation string) ([]byte, error) {
	b = append(b, `{"position":`...)
	open := len(b) - len(`"position":`)
	b, err := appendPosition(b, res.Position)
	if err != nil {
		return nil, err
	}
	if len(res.Captures) > 0 {
		b = appendStrings(appendMember(b, open, "captures"), res.Captures)
	}
	b = appendInt(appendMember(b, open, "next_seat"), res.NextSeat)
	if notation != "" {
		b = appendString(appendMember(b, open, "notation"), notation)
	}
	if len(res.Facts) > 0 {
		b = append(appendMember(b, open, "facts"), '[')
		for i, f := range res.Facts {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendString(append(b, `{"name":`...), f.Name)
			b = append(appendInt(append(b, `,"seat":`...), f.Seat), '}')
		}
		b = append(b, ']')
	}
	return append(appendOutcome(b, open, &res.Outcome), '}'), nil
}
