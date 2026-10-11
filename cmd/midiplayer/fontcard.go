package main

import (
	"fmt"
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"

	"github.com/marrasen/gunim-game-audio/soundfont"
)

// fontCard asks for a sound font while the SoundFont style has none: a
// button that downloads MuseScore General, saying how big it is and
// under what licence, and one that chooses a bank's file; and while the
// bank downloads, how far it has come. It rises into view as the style
// is chosen, and falls away once a bank plays.
type fontCard struct {
	l    *look
	size geom.Size
	// in is how far it has risen, and hot how lit each button is by
	// the pointer.
	in        float32
	hot       [2]float32
	over      int
	fetchBtn  geom.Rect
	chooseBtn geom.Rect
}

// wanted reports whether the card should show.
func (fc *fontCard) wanted() bool {
	s := fc.l.s
	return s.Style == "soundfont" && (s.Font == "" || s.Fetching >= 0)
}

// Step keeps the window drawing while the card rises or falls.
func (fc *fontCard) Step(time.Duration) bool {
	want := float32(0)
	if fc.wanted() {
		want = 1
	}
	return fc.in != want || fc.l.s.Fetching >= 0
}

func (fc *fontCard) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	fc.size = c.Max
	return c.Max
}

func (fc *fontCard) btnAt(p geom.Point) int {
	switch {
	case fc.l.s.Fetching >= 0:
		return -1
	case fc.fetchBtn.Contains(p):
		return 0
	case fc.chooseBtn.Contains(p):
		return 1
	}
	return -1
}

func (fc *fontCard) Handle(e input.Event, u *gunim.UI) bool {
	if fc.in < 0.5 {
		return false
	}
	switch e := e.(type) {
	case input.PointerMove:
		if o := fc.btnAt(e.Pos); o != fc.over {
			fc.over = o
			u.Invalidate()
		}
	case input.PointerLeave:
		fc.over = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return true
		}
		switch fc.btnAt(e.Pos) {
		case 0:
			u.Send(fc, FetchAsked{})
		case 1:
			u.Send(fc, FontOpenAsked{})
		}
	case input.PointerUp, input.Scroll:
	default:
		return false
	}
	// The card takes the pointer over all of it, so a click on it does
	// not fall through to the stage.
	return true
}

func (fc *fontCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := fc.l
	s := l.s
	pal := l.pal
	dt := float32(f.Delta.Seconds())
	want := float32(0)
	if fc.wanted() {
		want = 1
	}
	fc.in += (want - fc.in) * min(1, dt*8)
	if abs32f(want-fc.in) < 0.002 {
		fc.in = want
	}
	for i := range fc.hot {
		h := float32(0)
		if i == fc.over {
			h = 1
		}
		fc.hot[i] += (h - fc.hot[i]) * min(1, dt*14)
	}
	if fc.in <= 0.01 {
		return
	}
	in := ease(fc.in)
	full := geom.Rect{Max: box.Point()}
	end := p.Layer(paint.LayerOpts{Bounds: full.Inset(geom.Uniform(-40)), Opacity: in})
	defer end()
	defer p.Push(paint.Translate(geom.Pt(0, 24*(1-in))))()
	walnut := paint.Fill{Gradient: &paint.Gradient{From: full.Min, To: full.Max, Start: rgb(0x4a2c18), End: rgb(0x24150c)}}
	p.ShadowRRect(full, 20, walnut, paint.Shadow{Blur: 40, Offset: geom.Pt(0, 12), Color: alpha(color.NRGBA{A: 255}, 0.6)})
	p.RRectStroke(full, 20, paint.Fill{}, paint.Stroke{Width: 1.5, Color: alpha(rgb(0xd9b46a), 0.6)})
	ivory, brass := rgb(0xf6ecdc), rgb(0xe6b45a)
	// A row of ivory keys across the top, as the cartridge has.
	keys := xyxy(24, 22, box.W-24, 38)
	n := 21
	kw := keys.Size().W / float32(n)
	for k := range n {
		x := keys.Min.X + float32(k)*kw
		p.RRect(xyxy(x+0.5, keys.Min.Y, x+kw-0.5, keys.Max.Y), 1.5, paint.Solid(alpha(ivory, 0.85)))
	}
	title := shaped("Play on a sound font", 22, true)
	title.Paint(p, geom.Pt(28, 52), ivory)
	body := "A SoundFont bank holds recorded instruments, for MIDI files to sound real. None is loaded yet."
	fit(body, 13, false, box.W-56).Paint(p, geom.Pt(28, 86), alpha(ivory, 0.75))
	bank := soundfont.Banks[0]
	bw, bh := (box.W-56-14)/2, float32(52)
	fc.fetchBtn = xyxy(28, 116, 28+bw, 116+bh)
	fc.chooseBtn = xyxy(28+bw+14, 116, box.W-28, 116+bh)
	if f := s.Fetching; f >= 0 {
		// The download, as it comes.
		bar := xyxy(28, 128, box.W-28, 140)
		p.RRect(bar, 6, paint.Solid(alpha(color.NRGBA{A: 255}, 0.35)))
		p.RRect(xyxy(bar.Min.X, bar.Min.Y, bar.Min.X+bar.Size().W*float32(f), bar.Max.Y), 6, paint.Solid(brass))
		msg := shaped(fmt.Sprintf("Downloading %s…  %.0f%%", bank.Name, f*100), 13, true)
		msg.Paint(p, geom.Pt(28, 148), ivory)
	} else {
		fc.button(p, 0, fc.fetchBtn, icon.Download, "Download "+bank.Name,
			fmt.Sprintf("%d MB  ·  %s licence", bank.Size>>20, bank.License), brass, rgb(0x2a180c))
		fc.button(p, 1, fc.chooseBtn, icon.FolderOpen, "Choose a bank…", ".sf2 or .sf3, on this computer",
			alpha(ivory, 0.12), ivory)
	}
	foot := "Or drop an .sf2 or .sf3 file anywhere on the window.  The bank is kept for the next time."
	fit(foot, 11, false, box.W-56).Paint(p, geom.Pt(28, box.H-30), alpha(ivory, 0.55))
	_ = pal
}

// button draws button i in r: its icon, its label and the line under.
func (fc *fontCard) button(p *paint.Painter, i int, r geom.Rect, ic *icon.Icon, label, sub string, fill, ink color.NRGBA) {
	hot := fc.hot[i]
	mid := geom.Pt(r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2)
	defer p.Push(paint.Scale(1+0.03*hot, mid))()
	p.ShadowRRect(r, 12, paint.Solid(lighten(fill, 0.12*hot)), paint.Shadow{Blur: 8 + 10*hot, Offset: geom.Pt(0, 3), Color: alpha(color.NRGBA{A: 255}, 0.4)})
	p.Mask(icon.Stroke{Icon: ic, Width: 2, Progress: 1}, xyxy(r.Min.X+14, mid.Y-10, r.Min.X+34, mid.Y+10), ink)
	fit(label, 14, true, r.Size().W-56).Paint(p, geom.Pt(r.Min.X+44, r.Min.Y+9), ink)
	fit(sub, 11, false, r.Size().W-56).Paint(p, geom.Pt(r.Min.X+44, r.Min.Y+29), alpha(ink, 0.7))
}
