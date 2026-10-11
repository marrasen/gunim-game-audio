package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"

	"github.com/marrasen/gunim-game-audio/synth"
)

// band is the song's channels as a band on a stage: a station for each
// channel that plays, its instrument drawn and playing what it plays.
// A click on a station mutes it, or lets it sound again; a click with
// Shift plays it alone, or the whole band again.
//
// With no song it invites a drop, and offers Open and the demo.
type band struct {
	l      *look
	bounds geom.Rect
	size   geom.Size
	// rects are where the stations are, in the band's space, and chs the
	// channel of each.
	rects []geom.Rect
	chs   []int
	// hover is the station under the pointer, -1 for none, and press the
	// one pressed; hot is how lit each channel's station is by hover.
	hover, press int
	hot          [16]float32
	// The empty band's buttons, and which the pointer is over.
	openBtn, demoBtn geom.Rect
	overBtn          int
}

func (b *band) reset() { b.press = -1 }

func (b *band) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	b.size = c.Max
	b.rects, b.chs = b.rects[:0], b.chs[:0]
	for i := range b.l.station {
		b.l.station[i] = geom.Rect{}
	}
	sc := b.l.score
	if sc == nil {
		return c.Max
	}
	chs := sc.used()
	n := len(chs)
	if n == 0 {
		return c.Max
	}
	// As many columns as make the stations nearest a shape of 4 by 3.
	w, h := c.Max.W, c.Max.H
	const gap = 12
	best, cols := float32(math.Inf(1)), 1
	for k := 1; k <= n; k++ {
		rows := (n + k - 1) / k
		cw := (w - gap*float32(k-1)) / float32(k)
		ch := (h - gap*float32(rows-1)) / float32(rows)
		if cw < 120 || ch < 90 {
			continue
		}
		if d := float32(math.Abs(math.Log(float64(cw / ch / 1.45)))); d < best {
			best, cols = d, k
		}
	}
	rows := (n + cols - 1) / cols
	cw := (w - gap*float32(cols-1)) / float32(cols)
	chh := (h - gap*float32(rows-1)) / float32(rows)
	for i, ch := range chs {
		row, col := i/cols, i%cols
		// The last row is centred, where it is short.
		inRow := min(cols, n-row*cols)
		off := (float32(cols-inRow) * (cw + gap)) / 2
		x := off + float32(col)*(cw+gap)
		y := float32(row) * (chh + gap)
		rc := xyxy(x, y, x+cw, y+chh)
		b.rects = append(b.rects, rc)
		b.chs = append(b.chs, ch)
		b.l.station[ch] = rc.Add(b.bounds.Min)
	}
	return c.Max
}

// stationAt returns the station at pos, -1 for none.
func (b *band) stationAt(pos geom.Point) int {
	for i, rc := range b.rects {
		if rc.Contains(pos) {
			return i
		}
	}
	return -1
}

func (b *band) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if b.l.score == nil {
			was := b.overBtn
			b.overBtn = 0
			if b.openBtn.Contains(e.Pos) {
				b.overBtn = 1
			} else if b.demoBtn.Contains(e.Pos) {
				b.overBtn = 2
			}
			if was != b.overBtn {
				u.Invalidate()
			}
			return false
		}
		if h := b.stationAt(e.Pos); h != b.hover {
			b.hover = h
			u.Invalidate()
		}
	case input.PointerLeave:
		b.hover, b.overBtn = -1, 0
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if b.l.score == nil {
			switch {
			case b.openBtn.Contains(e.Pos):
				u.Send(b, OpenAsked{})
			case b.demoBtn.Contains(e.Pos):
				u.Send(b, DemoAsked{})
			default:
				return false
			}
			return true
		}
		b.press = b.stationAt(e.Pos)
		return b.press >= 0
	case input.PointerUp:
		if b.press >= 0 && b.press == b.stationAt(e.Pos) && b.press < len(b.chs) {
			u.Send(b, ChannelClicked{Channel: b.chs[b.press], Solo: e.Mods&input.ModShift != 0})
		}
		b.press = -1
	default:
		return false
	}
	return true
}

func (b *band) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := b.l
	dt := float32(f.Delta.Seconds())
	for i := range b.hot {
		want := float32(0)
		if b.hover >= 0 && b.hover < len(b.chs) && b.chs[b.hover] == i {
			want = 1
		}
		b.hot[i] += (want - b.hot[i]) * min(1, dt*12)
	}
	if l.score == nil {
		b.paintEmpty(p, box)
		return
	}
	for i, rc := range b.rects {
		// The stations pop in one after another as a song loads.
		in := ease(l.fresh*2.2 - float32(i)*0.05)
		if in <= 0 {
			continue
		}
		ch := b.chs[i]
		mid := geom.Pt(rc.Min.X+rc.Size().W/2, rc.Min.Y+rc.Size().H/2)
		sc := 0.85 + 0.15*in + 0.025*l.ch[ch].hit + 0.015*b.hot[ch]
		end := p.Push(paint.Scale(sc, mid))
		b.paintStation(p, rc, ch, in)
		end()
	}
}

// paintStation draws channel ch's station in rc: its card, its
// instrument playing, and its name.
func (b *band) paintStation(p *paint.Painter, rc geom.Rect, ch int, in float32) {
	l := b.l
	pal := l.pal
	s := l.s
	live := &l.ch[ch]
	col := pal.ch[ch]
	muted := s.Muted[ch] || s.Solo >= 0 && s.Solo != ch
	rad := 18 * pal.round
	fade := in
	if muted {
		fade *= 0.4
	}
	end := p.Layer(paint.LayerOpts{Bounds: rc.Inset(geom.Uniform(-30)), Opacity: fade})
	// The card, lit from below in the channel's colour as it plays.
	lit := min(1, live.level*0.8+live.hit*0.4)
	p.ShadowRRect(rc, rad, paint.Fill{Gradient: &paint.Gradient{From: rc.Min, To: geom.Pt(rc.Min.X, rc.Max.Y),
		Start: lighten(pal.panel, 0.04+0.05*b.hot[ch]), End: mix(pal.panel, col, 0.12+0.18*lit)}},
		paint.Shadow{Blur: 8 + 22*lit, Offset: geom.Pt(0, 4), Color: alpha(mix(color.NRGBA{A: 255}, col, lit), 0.35+0.3*lit)})
	edge := mix(pal.edge, col, 0.3+0.7*lit)
	width := float32(1)
	if s.Solo == ch {
		edge, width = pal.accent, 2.5
	}
	p.RRectStroke(rc, rad, paint.Fill{}, paint.Stroke{Width: width, Color: alpha(edge, 0.6+0.4*lit)})
	// The name: the channel's number on its colour, and the instrument.
	prog := live.program
	name := synth.GMNames[prog&127]
	if l.score.Channels[ch].Drums {
		name = "Drum kit"
	}
	badge := shaped(fmt.Sprint(ch+1), 11, true)
	bw := max(badge.Advance+10, 20)
	bx, by := rc.Min.X+10, rc.Min.Y+10
	p.RRect(xyxy(bx, by, bx+bw, by+18), 9*pal.round, paint.Solid(col))
	badge.Paint(p, geom.Pt(bx+(bw-badge.Advance)/2, by+2), darken(col, 0.75))
	fit(name, 12, true, rc.Size().W-bw-30).Paint(p, geom.Pt(bx+bw+8, by+2), alpha(pal.ink, 0.9))
	// The level, a bar along the bottom.
	lv := min(1, live.level)
	bar := xyxy(rc.Min.X+12, rc.Max.Y-9, rc.Max.X-12, rc.Max.Y-6)
	p.RRect(bar, 1.5*pal.round, paint.Solid(alpha(pal.edge, 0.4)))
	p.RRect(xyxy(bar.Min.X, bar.Min.Y, bar.Min.X+bar.Size().W*lv, bar.Max.Y), 1.5*pal.round, paint.Solid(col))
	// The instrument, playing.
	art := xyxy(rc.Min.X+10, rc.Min.Y+34, rc.Max.X-10, rc.Max.Y-16)
	if art.Size().H > 20 && art.Size().W > 40 {
		clip := p.Layer(paint.LayerOpts{Bounds: art.Inset(geom.Uniform(-6)), Opacity: 1, Clip: true, Radius: 10 * pal.round})
		drawInstrument(p, pen{pal: pal, col: col, t: l.t, live: live, info: l.score.Channels[ch], prog: prog, drums: l.score.Channels[ch].Drums}, art)
		clip()
	}
	if muted {
		m := shaped("MUTED", 13, true)
		if s.Solo >= 0 && s.Solo != ch && !s.Muted[ch] {
			m = shaped("SOLO ELSEWHERE", 11, true)
		}
		mx, my := rc.Min.X+rc.Size().W/2-m.Advance/2, rc.Min.Y+rc.Size().H/2-8
		p.RRect(xyxy(mx-10, my-4, mx+m.Advance+10, my+m.Height()+4), 6*pal.round, paint.Solid(alpha(pal.bgBottom, 0.85)))
		m.Paint(p, geom.Pt(mx, my), pal.ink)
	}
	end()
}

// paintEmpty invites a drop: a slot that glows and breathes, notes
// bobbing over it, and the buttons to open files or play the demo.
func (b *band) paintEmpty(p *paint.Painter, box geom.Size) {
	pal := b.l.pal
	t := float64(b.l.now.UnixMilli()%1000000) / 1000
	cx, cy := box.W/2, box.H/2-10
	breathe := 0.5 + 0.5*sinf(t*1.6)
	slot := xyxy(cx-170, cy-90, cx+170, cy+60)
	p.ShadowRRect(slot, 26*pal.round, paint.Solid(alpha(pal.panel, 0.7)),
		paint.Shadow{Blur: 30 + 20*breathe, Color: alpha(pal.accent, 0.25+0.2*breathe)})
	// A dashed border, its dashes marching round.
	dash := float32(14)
	off := float32(math.Mod(t*30, float64(dash*2)))
	per := 2 * (slot.Size().W + slot.Size().H)
	for d := -off; d < per; d += dash * 2 {
		a, z := max(d, 0), min(d+dash, per)
		if z <= a {
			continue
		}
		b.dashAlong(p, slot.Inset(geom.Uniform(8)), a/per, z/per, alpha(pal.accent, 0.55))
	}
	// Notes bobbing in the slot, each in a channel's colour.
	glyphs := []string{"♪", "♫", "♩", "♬"}
	for i := range 7 {
		x := cx - 120 + float32(i)*40
		y := cy - 30 + 14*sinf(t*2.2+float64(i)*0.9)
		g := shaped(glyphs[i%4], 30, true)
		g.Paint(p, geom.Pt(snap(x-g.Advance/2, pal.pixel), snap(y-20, pal.pixel)), alpha(pal.ch[i], 0.85))
	}
	msg := shaped("Drop MIDI files here", 22, true)
	msg.Paint(p, geom.Pt(cx-msg.Advance/2, cy+18), pal.ink)
	// The two buttons under the slot.
	bw, bh := float32(150), float32(40)
	b.openBtn = xyxy(cx-bw-8, slot.Max.Y+24, cx-8, slot.Max.Y+24+bh)
	b.demoBtn = xyxy(cx+8, slot.Max.Y+24, cx+8+bw, slot.Max.Y+24+bh)
	for i, btn := range []geom.Rect{b.openBtn, b.demoBtn} {
		label := []string{"Open files", "Play the demo"}[i]
		over := b.overBtn == i+1
		fill := alpha(pal.panel, 0.9)
		ink := pal.ink
		if i == 1 {
			fill, ink = pal.accent, darken(pal.accent, 0.8)
		}
		if over {
			fill = lighten(fill, 0.15)
		}
		p.ShadowRRect(btn, bh/2*pal.round, paint.Solid(fill), paint.Shadow{Blur: 12, Offset: geom.Pt(0, 3), Color: alpha(color.NRGBA{A: 255}, 0.4)})
		if i == 0 {
			p.RRectStroke(btn, bh/2*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: pal.edge})
		}
		run := shaped(label, 14, true)
		run.Paint(p, geom.Pt(btn.Min.X+(bw-run.Advance)/2, btn.Min.Y+(bh-run.Height())/2), ink)
	}
	hint := shaped("Space plays · ← → move · N P skip · 1–5 pick a style · L the playlist", 12, false)
	hint.Paint(p, geom.Pt(cx-hint.Advance/2, b.openBtn.Max.Y+22), alpha(pal.dim, 0.8))
}

// dashAlong draws the stretch of rc's edge from a to z, as fractions of
// the way round it from its top left, clockwise.
func (b *band) dashAlong(p *paint.Painter, rc geom.Rect, a, z float32, c color.NRGBA) {
	w, h := rc.Size().W, rc.Size().H
	per := 2 * (w + h)
	at := func(d float32) geom.Point {
		d *= per
		switch {
		case d < w:
			return geom.Pt(rc.Min.X+d, rc.Min.Y)
		case d < w+h:
			return geom.Pt(rc.Max.X, rc.Min.Y+d-w)
		case d < 2*w+h:
			return geom.Pt(rc.Max.X-(d-w-h), rc.Max.Y)
		}
		return geom.Pt(rc.Min.X, rc.Max.Y-(d-2*w-h))
	}
	p0, p1 := at(a), at(z)
	if p0.X != p1.X && p0.Y != p1.Y {
		// Round a corner: draw the dash's start alone.
		p1 = p0
	}
	r := xyxy(min(p0.X, p1.X)-1.5, min(p0.Y, p1.Y)-1.5, max(p0.X, p1.X)+1.5, max(p0.Y, p1.Y)+1.5)
	p.RRect(r, 1.5, paint.Solid(c))
}
