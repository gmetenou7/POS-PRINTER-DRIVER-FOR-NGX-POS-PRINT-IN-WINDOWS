// Package escpos builds raw ESC/POS command streams that work across
// virtually every thermal receipt printer that speaks the Epson ESC/POS dialect.
//
// All values are conservative: the builder targets the lowest common
// denominator that we observe across Epson TM, Star (in ESC/POS mode),
// Bixolon, Citizen, XPrinter, HPRT, and the generic Chinese clones.
package escpos

import "bytes"

const (
	ESC byte = 0x1B
	GS  byte = 0x1D
	LF  byte = 0x0A
	FF  byte = 0x0C
	CR  byte = 0x0D
)

type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

type Builder struct {
	buf bytes.Buffer
}

func New() *Builder {
	b := &Builder{}
	b.Init()
	return b
}

// Init resets the printer to default state (ESC @).
func (b *Builder) Init() *Builder {
	b.buf.Write([]byte{ESC, '@'})
	return b
}

func (b *Builder) Text(s string) *Builder {
	b.buf.WriteString(s)
	return b
}

func (b *Builder) Line(s string) *Builder {
	b.buf.WriteString(s)
	b.buf.WriteByte(LF)
	return b
}

func (b *Builder) Feed(n int) *Builder {
	if n <= 0 {
		n = 1
	}
	if n > 255 {
		n = 255
	}
	b.buf.Write([]byte{ESC, 'd', byte(n)})
	return b
}

func (b *Builder) Bold(on bool) *Builder {
	v := byte(0)
	if on {
		v = 1
	}
	b.buf.Write([]byte{ESC, 'E', v})
	return b
}

func (b *Builder) Underline(level int) *Builder {
	if level < 0 {
		level = 0
	}
	if level > 2 {
		level = 2
	}
	b.buf.Write([]byte{ESC, '-', byte(level)})
	return b
}

func (b *Builder) Align(a Align) *Builder {
	b.buf.Write([]byte{ESC, 'a', byte(a)})
	return b
}

// Size sets horizontal+vertical magnification (1..8). 1 = normal.
func (b *Builder) Size(w, h int) *Builder {
	clamp := func(v int) int {
		if v < 1 {
			return 1
		}
		if v > 8 {
			return 8
		}
		return v
	}
	v := byte((clamp(w)-1)<<4 | (clamp(h) - 1))
	b.buf.Write([]byte{GS, '!', v})
	return b
}

// Cut performs a full paper cut. Falls back to partial-cut command form
// that the vast majority of clones accept.
func (b *Builder) Cut() *Builder {
	// Feed a few lines first so the cut doesn't slice into content.
	b.Feed(3)
	b.buf.Write([]byte{GS, 'V', 0x00}) // full cut
	return b
}

// PartialCut leaves a small uncut tab so the receipt stays attached.
func (b *Builder) PartialCut() *Builder {
	b.Feed(3)
	b.buf.Write([]byte{GS, 'V', 0x01}) // partial cut
	return b
}

// OpenDrawer triggers the cash drawer kick on pin 2 (most common).
func (b *Builder) OpenDrawer() *Builder {
	b.buf.Write([]byte{ESC, 'p', 0x00, 0x32, 0xFA})
	return b
}

// Bytes returns the built command stream.
func (b *Builder) Bytes() []byte {
	return b.buf.Bytes()
}

// PlainText is a convenience: builds a ticket that prints `text` then cuts.
// Newlines in `text` are honored. Useful for /print quick-mode.
func PlainText(text string, cut bool) []byte {
	b := New()
	b.Align(AlignLeft)
	b.Line(text)
	if cut {
		b.Cut()
	}
	return b.Bytes()
}
