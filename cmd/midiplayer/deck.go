package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// deck is the styles as the hardware that made each sound: a gold
// score for General MIDI, a Commodore 64's cartridge, a NES cartridge,
// a Game Boy's, and an AdLib card. The style chosen is pushed down into
// its slot and lit; the others stand up out of theirs, and lift as the
// pointer passes.
type deck struct {
	l    *look
	size geom.Size
	// in is how far down each cartridge sits in its slot, easing, and
	// hot how lit it is by hover.
	in, hot [6]float32
	over    int
	down    int
	laid    bool
}

func (d *deck) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	d.size = c.Max
	return c.Max
}

// cart returns where cartridge i stands, before it moves.
func (d *deck) cart(i int) geom.Rect {
	w := d.size.W / float32(len(styleOrder))
	return xyxy(float32(i)*w+5, 4, float32(i+1)*w-5, d.size.H-14)
}

func (d *deck) cartAt(pos geom.Point) int {
	for i := range styleOrder {
		if d.cart(i).Contains(pos) {
			return i
		}
	}
	return -1
}

func (d *deck) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if o := d.cartAt(e.Pos); o != d.over {
			d.over = o
			u.Invalidate()
		}
		return false
	case input.PointerLeave:
		d.over = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		d.down = d.cartAt(e.Pos)
		return d.down >= 0
	case input.PointerUp:
		if i := d.cartAt(e.Pos); i >= 0 && i == d.down {
			u.Send(d, StyleChosen{Style: styleOrder[i]})
		}
		d.down = -1
	default:
		return false
	}
	return true
}

func (d *deck) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	pal := d.l.pal
	dt := float32(f.Delta.Seconds())
	for i, st := range styleOrder {
		want := float32(0)
		if d.l.s.Style == st {
			want = 1
		}
		if !d.laid {
			d.in[i] = want
		}
		// The cartridge chosen drops in with a little bounce.
		d.in[i] += (want - d.in[i]) * min(1, dt*9)
		h := float32(0)
		if i == d.over {
			h = 1
		}
		d.hot[i] += (h - d.hot[i]) * min(1, dt*12)
	}
	d.laid = true
	// The slot they stand in.
	slot := xyxy(0, box.H-22, box.W, box.H-2)
	p.RRect(slot, 8*pal.round, paint.Solid(alpha(darken(pal.panel, 0.4), 0.9)))
	p.RRectStroke(slot, 8*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.6)})
	for i := range styleOrder {
		d.paintCart(p, i, pal)
	}
	// A light along the slot under the one chosen.
	for i := range styleOrder {
		if d.in[i] > 0.05 {
			r := d.cart(i)
			func() {
				defer p.Blend(paint.BlendAdd)()
				p.ShadowRRect(xyxy(r.Min.X+6, slot.Min.Y+2, r.Max.X-6, slot.Min.Y+5), 2, paint.Solid(alpha(pal.accent, d.in[i])),
					paint.Shadow{Blur: 14, Color: alpha(pal.accent, 0.7*d.in[i])})
			}()
		}
	}
}

// paintCart draws cartridge i, each after its machine.
func (d *deck) paintCart(p *paint.Painter, i int, pal palette) {
	r := d.cart(i)
	sink := d.in[i]*26 - d.hot[i]*6*(1-d.in[i])
	r = r.Add(geom.Pt(0, sink))
	w, h := r.Size().W, r.Size().H
	end := p.Layer(paint.LayerOpts{Bounds: xyxy(r.Min.X-20, -20, r.Max.X+20, d.size.H-20), Opacity: 1, Clip: true})
	defer end()
	shadow := paint.Shadow{Blur: 10 + 8*d.hot[i], Offset: geom.Pt(0, 4), Color: alpha(color.NRGBA{A: 255}, 0.45)}
	label := func(rc geom.Rect, fill color.NRGBA, title string, ink color.NRGBA, size float32) {
		p.RRect(rc, 3, paint.Solid(fill))
		run := fit(title, size, true, rc.Size().W-6)
		run.Paint(p, geom.Pt(rc.Min.X+(rc.Size().W-run.Advance)/2, rc.Min.Y+(rc.Size().H-run.Height())/2), ink)
	}
	switch styleOrder[i] {
	case "gm":
		// A gold-edged score, a treble clef on it.
		p.ShadowRRect(r, 8, paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: r.Max, Start: rgb(0x2b2350), End: rgb(0x15112c)}}, shadow)
		p.RRectStroke(r.Inset(geom.Uniform(3)), 6, paint.Fill{}, paint.Stroke{Width: 1.5, Color: rgb(0xffc35c)})
		for k := range 5 {
			y := r.Min.Y + 14 + float32(k)*5
			p.RRect(xyxy(r.Min.X+10, y, r.Max.X-10, y+1), 0, paint.Solid(alpha(rgb(0xffc35c), 0.5)))
		}
		cl := shaped("𝄞", 30, false)
		if cl.Advance < 2 {
			cl = shaped("♫", 24, true)
		}
		cl.Paint(p, geom.Pt(r.Min.X+12, r.Min.Y+4), rgb(0xffe2a0))
		label(xyxy(r.Min.X+6, r.Max.Y-24, r.Max.X-6, r.Max.Y-8), rgb(0xffc35c), "ORCHESTRA", rgb(0x2b2350), 10)
	case "sid":
		// The C64's brown cartridge, its rainbow stripe.
		p.ShadowRRect(r, 4, paint.Solid(rgb(0x8a7a5e)), shadow)
		p.RRect(xyxy(r.Min.X+4, r.Min.Y+4, r.Max.X-4, r.Min.Y+8), 1, paint.Solid(rgb(0x6e6048)))
		stripes := []uint32{0xd03030, 0xe08a20, 0xe8d040, 0x40a040, 0x3060c0}
		for k, c := range stripes {
			y := r.Min.Y + 16 + float32(k)*4
			p.RRect(xyxy(r.Min.X+8, y, r.Max.X-8, y+3), 0, paint.Solid(rgb(c)))
		}
		label(xyxy(r.Min.X+6, r.Max.Y-24, r.Max.X-6, r.Max.Y-8), rgb(0x40318d), "C64 SID", rgb(0xa59fef), 10)
	case "nes":
		// The NES's grey cartridge, its ridges and its red label.
		p.ShadowRRect(r, 3, paint.Solid(rgb(0x9a9a9a)), shadow)
		for k := range 4 {
			y := r.Min.Y + 6 + float32(k)*4
			p.RRect(xyxy(r.Min.X+6, y, r.Max.X-6, y+2), 0, paint.Solid(rgb(0x7a7a7a)))
		}
		label(xyxy(r.Min.X+7, r.Min.Y+26, r.Max.X-7, r.Max.Y-8), rgb(0xd81e1e), "NES", rgb(0xffffff), 13)
	case "gb":
		// The Game Boy's little cartridge: a notch at its corner.
		gr := xyxy(r.Min.X+w*0.1, r.Min.Y, r.Max.X-w*0.1, r.Max.Y)
		p.ShadowRRect(gr, 4, paint.Solid(rgb(0xb8b8b4)), shadow)
		p.RRect(xyxy(gr.Max.X-8, gr.Min.Y, gr.Max.X, gr.Min.Y+8), 0, paint.Solid(alpha(pal.panel, 0)))
		for k := range 3 {
			y := gr.Min.Y + 5 + float32(k)*3
			p.RRect(xyxy(gr.Min.X+5, y, gr.Max.X-10, y+1.5), 0, paint.Solid(rgb(0x989894)))
		}
		lab := xyxy(gr.Min.X+5, gr.Min.Y+18, gr.Max.X-5, gr.Max.Y-8)
		p.RRect(lab, 3, paint.Solid(rgb(0x8bac0f)))
		run := fit("GAME BOY", 9, true, lab.Size().W-4)
		run.Paint(p, geom.Pt(lab.Min.X+(lab.Size().W-run.Advance)/2, lab.Min.Y+(lab.Size().H-run.Height())/2), rgb(0x0f380f))
	case "adlib":
		// A green card, its chips, and its gold fingers.
		p.ShadowRRect(r, 2, paint.Solid(rgb(0x1d6b3a)), shadow)
		p.RRect(xyxy(r.Min.X+8, r.Min.Y+8, r.Min.X+w*0.5, r.Min.Y+22), 1, paint.Solid(rgb(0x151515)))
		p.RRect(xyxy(r.Min.X+w*0.58, r.Min.Y+10, r.Max.X-8, r.Min.Y+18), 1, paint.Solid(rgb(0x151515)))
		for k := range 6 {
			x := r.Min.X + 8 + float32(k)*(w-16)/6
			p.RRect(xyxy(x, r.Max.Y-8, x+(w-16)/6-2, r.Max.Y-2), 0, paint.Solid(rgb(0xe0b040)))
		}
		run := fit("AdLib FM", 10, true, w-8)
		run.Paint(p, geom.Pt(r.Min.X+(w-run.Advance)/2, r.Min.Y+28), rgb(0xe8f0e0))
	case "soundfont":
		// A walnut card with ivory keys, and the bank's name, as a sample
		// player's might be.
		p.ShadowRRect(r, 6, paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: r.Max, Start: rgb(0x6b3f22), End: rgb(0x3a2212)}}, shadow)
		keys := xyxy(r.Min.X+6, r.Min.Y+8, r.Max.X-6, r.Min.Y+30)
		n := 7
		kw := keys.Size().W / float32(n)
		for k := range n {
			x := keys.Min.X + float32(k)*kw
			p.RRect(xyxy(x+0.5, keys.Min.Y, x+kw-0.5, keys.Max.Y), 1.5, paint.Solid(rgb(0xf4efe2)))
		}
		for k := range n - 1 {
			if k == 2 {
				continue
			}
			x := keys.Min.X + float32(k+1)*kw
			p.RRect(xyxy(x-kw*0.3, keys.Min.Y, x+kw*0.3, keys.Min.Y+13), 1, paint.Solid(rgb(0x1a1410)))
		}
		name := d.l.s.Font
		if name == "" {
			name = "NO BANK"
		}
		label(xyxy(r.Min.X+5, r.Max.Y-24, r.Max.X-5, r.Max.Y-8), rgb(0xd9b46a), "SOUNDFONT", rgb(0x3a2212), 9)
		fit(name, 8, false, w-10).Paint(p, geom.Pt(r.Min.X+6, r.Min.Y+33), alpha(rgb(0xf4efe2), 0.85))
		if f := d.l.s.Fetching; f >= 0 {
			bar := xyxy(r.Min.X+6, r.Min.Y+44, r.Max.X-6, r.Min.Y+48)
			p.RRect(bar, 2, paint.Solid(alpha(rgb(0x000000), 0.4)))
			p.RRect(xyxy(bar.Min.X, bar.Min.Y, bar.Min.X+bar.Size().W*float32(f), bar.Max.Y), 2, paint.Solid(rgb(0xd9b46a)))
		}
	}
	// The cartridge chosen glows from within.
	if d.in[i] > 0.05 {
		defer p.Blend(paint.BlendAdd)()
		p.ShadowRRect(r.Inset(geom.Uniform(6)), 4, paint.Solid(color.NRGBA{}), paint.Shadow{Blur: 18, Color: alpha(pal.accent, 0.35*d.in[i])})
	}
	_ = h
}
