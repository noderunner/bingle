// Package qr draws a QR code: just enough of ISO/IEC 18004 for the
// otpauth:// link a second factor is set up with. It encodes bytes (byte
// mode) at error correction level M in versions 1 to 10, up to 213 bytes,
// and draws an SVG. It is written here rather than imported because it is
// small, has one caller, and the dependency count is a feature (ADR 0074).
package qr

import (
	"errors"
	"fmt"
	"strings"
)

// MaxBytes is the most Encode takes: version 10 at level M.
const MaxBytes = 213

// ErrTooLong is Encode's answer to more than MaxBytes.
var ErrTooLong = errors.New("qr: too much to encode")

// Code is a drawn QR code: Size by Size modules, true is dark.
type Code struct {
	Version int
	Size    int
	Mask    int
	dark    [][]bool
	fn      [][]bool // while drawing: the modules data may not use
}

// Dark reports whether the module at column x, row y is dark.
func (c *Code) Dark(x, y int) bool { return c.dark[y][x] }

// Level M's error correction, by version: codewords per block and blocks.
var (
	eccPerBlock = [11]int{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26}
	eccBlocks   = [11]int{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5}
	// alignment are the centres of the alignment patterns, by version.
	alignment = [11][]int{nil, nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34},
		{6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}}
)

// rawModules is how many modules of a version carry data and error
// correction: everything but the function patterns.
func rawModules(v int) int {
	n := (16*v+128)*v + 64
	if v >= 2 {
		align := v/7 + 2
		n -= (25*align-10)*align - 55
		if v >= 7 {
			n -= 36
		}
	}
	return n
}

// dataCodewords is how many data codewords a version holds at level M.
func dataCodewords(v int) int { return rawModules(v)/8 - eccPerBlock[v]*eccBlocks[v] }

// Encode draws data as the smallest QR code that holds it.
func Encode(data []byte) (*Code, error) {
	v := 1
	for ; v <= 10; v++ {
		countBits := 8
		if v >= 10 {
			countBits = 16
		}
		if 4+countBits+8*len(data) <= 8*dataCodewords(v) {
			break
		}
	}
	if v > 10 {
		return nil, ErrTooLong
	}
	return draw(v, data, -1), nil
}

// draw makes the code of a version with a mask, or the best mask for -1.
func draw(v int, data []byte, mask int) *Code {
	c := newCode(v)
	c.placeData(interleave(v, encodeData(v, data)))
	if mask < 0 {
		c.chooseMask()
	} else {
		c.applyMask(mask)
		c.drawFormat(mask)
		c.Mask = mask
		c.fn = nil
	}
	return c
}

// ---- the data -----------------------------------------------------------------

type bitBuffer struct{ bits []bool }

func (b *bitBuffer) put(value, n int) {
	for i := n - 1; i >= 0; i-- {
		b.bits = append(b.bits, value>>i&1 == 1)
	}
}

// encodeData is the data codewords: byte mode, the count, the bytes, the
// terminator and the padding.
func encodeData(v int, data []byte) []byte {
	var b bitBuffer
	b.put(0b0100, 4)
	if v >= 10 {
		b.put(len(data), 16)
	} else {
		b.put(len(data), 8)
	}
	for _, d := range data {
		b.put(int(d), 8)
	}
	capacity := 8 * dataCodewords(v)
	b.put(0, min(4, capacity-len(b.bits)))
	b.put(0, (8-len(b.bits)%8)%8)
	for pad := 0xEC; len(b.bits) < capacity; pad ^= 0xEC ^ 0x11 {
		b.put(pad, 8)
	}
	out := make([]byte, len(b.bits)/8)
	for i, bit := range b.bits {
		if bit {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

// interleave splits the data into the version's blocks, adds each block's
// error correction and interleaves them, as the symbol is read.
func interleave(v int, data []byte) []byte {
	blocks, ecc := eccBlocks[v], eccPerBlock[v]
	short := len(data) / blocks
	longs := len(data) % blocks // the last blocks hold one more
	gen := generator(ecc)
	var parts, eccs [][]byte
	at := 0
	for i := 0; i < blocks; i++ {
		n := short
		if i >= blocks-longs {
			n++
		}
		parts = append(parts, data[at:at+n])
		eccs = append(eccs, remainder(data[at:at+n], gen))
		at += n
	}
	var out []byte
	for i := 0; i <= short; i++ {
		for _, p := range parts {
			if i < len(p) {
				out = append(out, p[i])
			}
		}
	}
	for i := 0; i < ecc; i++ {
		for _, e := range eccs {
			out = append(out, e[i])
		}
	}
	return out
}

// ---- Reed-Solomon over GF(256), the polynomial 0x11D ----------------------------

func gfMul(a, b byte) byte {
	var r byte
	for ; b != 0; b >>= 1 {
		if b&1 != 0 {
			r ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1D
		}
	}
	return r
}

// generator is the product of (x - 2^i) for i below degree, highest power
// first, its leading 1 left out.
func generator(degree int) []byte {
	g := make([]byte, degree)
	g[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			g[j] = gfMul(g[j], root)
			if j+1 < degree {
				g[j] ^= g[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return g
}

// remainder is the error correction for data: data times x^degree, modulo
// the generator.
func remainder(data, gen []byte) []byte {
	r := make([]byte, len(gen))
	for _, d := range data {
		factor := d ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i := range r {
			r[i] ^= gfMul(gen[i], factor)
		}
	}
	return r
}

// ---- the symbol -------------------------------------------------------------------

func newCode(v int) *Code {
	size := 4*v + 17
	c := &Code{Version: v, Size: size, dark: grid(size)}
	fn := grid(size)
	set := func(x, y int, dark bool) { c.dark[y][x] = dark; fn[y][x] = true }
	for i := 0; i < size; i++ {
		set(6, i, i%2 == 0)
		set(i, 6, i%2 == 0)
	}
	for _, p := range [][2]int{{3, 3}, {size - 4, 3}, {3, size - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := p[0]+dx, p[1]+dy
				if x >= 0 && x < size && y >= 0 && y < size {
					d := max(abs(dx), abs(dy))
					set(x, y, d != 2 && d != 4)
				}
			}
		}
	}
	al := alignment[v]
	for i, ay := range al {
		for j, ax := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue // under a finder
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					set(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	// Reserve the format areas (drawn with the mask) and the dark module.
	for i := 0; i < 9; i++ {
		if i == 6 {
			continue // the timing patterns cross here, and stay
		}
		set(8, i, false)
		set(i, 8, false)
	}
	for i := 0; i < 8; i++ {
		set(size-1-i, 8, false)
		set(8, size-1-i, false)
	}
	set(8, size-8, true)
	if v >= 7 {
		rem := v
		for i := 0; i < 12; i++ {
			rem = rem<<1 ^ (rem>>11)*0x1F25
		}
		bits := v<<12 | rem
		for i := 0; i < 18; i++ {
			dark := bits>>i&1 == 1
			a, b := size-11+i%3, i/3
			set(a, b, dark)
			set(b, a, dark)
		}
	}
	c.fn = fn
	return c
}

// placeData lays the codewords in the zigzag from the bottom right corner,
// two columns at a time, skipping the vertical timing pattern.
func (c *Code) placeData(words []byte) {
	fn := c.fn
	i := 0
	for right := c.Size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vert := 0; vert < c.Size; vert++ {
			y := vert
			if upward {
				y = c.Size - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if fn[y][x] || i >= len(words)*8 {
					continue // remainder bits stay light
				}
				c.dark[y][x] = words[i/8]>>(7-i%8)&1 == 1
				i++
			}
		}
	}
}

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (c *Code) applyMask(mask int) {
	fn := c.fn
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if !fn[y][x] && maskBit(mask, x, y) {
				c.dark[y][x] = !c.dark[y][x]
			}
		}
	}
}

// formatBits is the 15-bit format information for level M and a mask.
func formatBits(mask int) int {
	data := 0b00<<3 | mask // level M is 00
	rem := data
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	return (data<<10 | rem) ^ 0x5412
}

func (c *Code) drawFormat(mask int) {
	bits := formatBits(mask)
	bit := func(i int) bool { return bits>>i&1 == 1 }
	n := c.Size
	for i := 0; i <= 5; i++ {
		c.dark[i][8] = bit(i)
	}
	c.dark[7][8] = bit(6)
	c.dark[8][8] = bit(7)
	c.dark[8][7] = bit(8)
	for i := 9; i < 15; i++ {
		c.dark[8][14-i] = bit(i)
	}
	for i := 0; i < 8; i++ {
		c.dark[8][n-1-i] = bit(i)
	}
	for i := 8; i < 15; i++ {
		c.dark[n-15+i][8] = bit(i)
	}
	c.dark[n-8][8] = true
}

// chooseMask tries the eight masks and keeps the one the standard's
// penalty likes best: any of them reads, this one reads most easily.
func (c *Code) chooseMask() {
	best, bestScore := 0, -1
	for m := 0; m < 8; m++ {
		c.applyMask(m)
		c.drawFormat(m)
		if s := c.penalty(); bestScore < 0 || s < bestScore {
			best, bestScore = m, s
		}
		c.applyMask(m) // undo
	}
	c.applyMask(best)
	c.drawFormat(best)
	c.Mask = best
	c.fn = nil
}

// penalty is the standard's score of how hard the symbol is to read.
func (c *Code) penalty() int {
	n := c.Size
	score := 0
	line := func(get func(i int) bool) {
		run := 1
		for i := 1; i <= n; i++ {
			if i < n && get(i) == get(i-1) {
				run++
				continue
			}
			if run >= 5 {
				score += 3 + run - 5
			}
			run = 1
		}
		// 1:1:3:1:1 with four light modules on one side, outside counts as light.
		at := func(i int) bool { return i >= 0 && i < n && get(i) }
		for i := -4; i < n; i++ {
			core := at(i) && !at(i+1) && at(i+2) && at(i+3) && at(i+4) && !at(i+5) && at(i+6)
			if !core {
				continue
			}
			before := !at(i-1) && !at(i-2) && !at(i-3) && !at(i-4)
			after := !at(i+7) && !at(i+8) && !at(i+9) && !at(i+10)
			if before || after {
				score += 40
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		line(func(x int) bool { return c.dark[y][x] })
		line(func(x int) bool { return c.dark[x][y] })
		for x := 0; x < n; x++ {
			if c.dark[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				d := c.dark[y][x]
				if d == c.dark[y][x+1] && d == c.dark[y+1][x] && d == c.dark[y+1][x+1] {
					score += 3
				}
			}
		}
	}
	// Ten for every five per cent the dark share is away from half.
	total := n * n
	score += 10 * (abs(20*dark-10*total) / total)
	return score
}

// SVG draws the code with a quiet zone of four modules, each module a unit
// of the viewBox, dark on light whatever the page's colours.
func (c *Code) SVG() string {
	var b strings.Builder
	q := 4
	w := c.Size + 2*q
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, w, w)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, w, w)
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.dark[y][x] {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+q, y+q)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

func grid(n int) [][]bool {
	g := make([][]bool, n)
	for i := range g {
		g[i] = make([]bool, n)
	}
	return g
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
