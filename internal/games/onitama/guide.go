package onitama

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

//go:embed guide
var guideFS embed.FS

// guideAssets is the module's guide: the embedded pages and pictures, plus
// the card pages, which are drawn from the card table so they can never
// disagree with the rules.
type guideAssets struct {
	base  fs.FS
	extra map[string][]byte
}

func newGuideAssets() fs.FS {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return guideAssets{base: sub, extra: map[string][]byte{
		"cards.md":  []byte(cardsMarkdown()),
		"cards.svg": []byte(cardsSheet()),
	}}
}

func (g guideAssets) Open(name string) (fs.File, error) {
	if data, ok := g.extra[name]; ok {
		return &memFile{name: path.Base(name), r: bytes.NewReader(data), size: int64(len(data))}, nil
	}
	return g.base.Open(name)
}

type memFile struct {
	name string
	r    *bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Name() string               { return f.name }
func (f *memFile) Size() int64                { return f.size }
func (f *memFile) Mode() fs.FileMode          { return 0o444 }
func (f *memFile) ModTime() time.Time         { return time.Time{} }
func (f *memFile) IsDir() bool                { return false }
func (f *memFile) Sys() any                   { return nil }

var _ io.Reader = (*memFile)(nil)

func title(id string) string {
	return strings.ToUpper(id[:1]) + id[1:]
}

// cardsMarkdown is the Cards page: the picture of every card and a list of
// each card's moves in words.
func cardsMarkdown() string {
	var b strings.Builder
	b.WriteString("# The Move cards\n\n")
	b.WriteString("The sixteen cards of the game. On a card the **black square** is your piece and the **coloured squares** are the squares it may move to, drawn the way the holder sees the board: **up is forwards**, towards the opponent. Blue's cards are used from the other side, so for Blue the same picture points down the board.\n\n")
	b.WriteString("![All sixteen Move cards](cards.svg)\n\n")
	b.WriteString("## In words\n\n| Card | Moves |\n|---|---|\n")
	for _, c := range cards {
		var parts []string
		for _, m := range c.Moves {
			parts = append(parts, describeMove(m))
		}
		fmt.Fprintf(&b, "| **%s** | %s |\n", title(c.ID), strings.Join(parts, "; "))
	}
	b.WriteString("\nA card's stamp (red or blue) is only used in the printed game, to decide who starts when the card is the one at the side. Bingle always lets Red begin.\n")
	return b.String()
}

func describeMove(m [2]int) string {
	right, fwd := m[0], m[1]
	var out []string
	switch {
	case fwd > 0:
		out = append(out, fmt.Sprintf("%d forward", fwd))
	case fwd < 0:
		out = append(out, fmt.Sprintf("%d back", -fwd))
	}
	switch {
	case right > 0:
		out = append(out, fmt.Sprintf("%d right", right))
	case right < 0:
		out = append(out, fmt.Sprintf("%d left", -right))
	}
	return strings.Join(out, " and ")
}

// cardsSheet draws every card as a small 5×5 grid, four cards across.
func cardsSheet() string {
	const (
		cw, ch   = 156, 176
		gap      = 12
		cell     = 22.0
		columns  = 4
		pad      = 12
		gridSize = 5 * cell
	)
	rows := (len(cards) + columns - 1) / columns
	w := pad*2 + columns*cw + (columns-1)*gap
	h := pad*2 + rows*ch + (rows-1)*gap
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="The sixteen Onitama Move cards" font-family="system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" rx="10" fill="#f7f0df"/>`, w, h)
	for i, c := range cards {
		x := float64(pad + (i%columns)*(cw+gap))
		y := float64(pad + (i/columns)*(ch+gap))
		stamp := "#c8372d"
		if c.Stamp == "blue" {
			stamp = "#2d5fc4"
		}
		fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%d" height="%d" rx="9" fill="#ecdcb2" stroke="#8a6a43" stroke-width="2"/>`, x, y, cw, ch)
		fmt.Fprintf(&b, `<text x="%g" y="%g" font-size="15" font-weight="800" fill="#2b2118" dominant-baseline="central">%s</text>`, x+12, y+19, title(c.ID))
		fmt.Fprintf(&b, `<circle cx="%g" cy="%g" r="6" fill="%s"/>`, x+cw-18, y+19, stamp)
		gx := x + (cw-gridSize)/2
		gy := y + 38
		for r := 0; r < 5; r++ {
			for col := 0; col < 5; col++ {
				fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%g" height="%g" fill="#f7f0df" stroke="#b9a57a"/>`, gx+float64(col)*cell, gy+float64(r)*cell, cell, cell)
			}
		}
		for _, m := range c.Moves {
			col, row := 2+m[0], 2-m[1] // forward is up
			fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%g" height="%g" fill="#4aa86a" stroke="#2b6a3f"/>`, gx+float64(col)*cell, gy+float64(row)*cell, cell, cell)
		}
		fmt.Fprintf(&b, `<rect x="%g" y="%g" width="%g" height="%g" fill="#2b2118"/>`, gx+2*cell+4, gy+2*cell+4, cell-8, cell-8)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
