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
)

// transport is the strip along the bottom: the song's timeline, a map of
// how busy each part of it is that a click or a drag moves through,
// and under it play and pause, skip, repeat, the time, the speed, the
// transpose, the volume, Open and the playlist.
type transport struct {
	l    *look
	list *playlist
	size geom.Size
	// timeline is where the timeline lies, and ctls the controls.
	timeline geom.Rect
	ctls     []ctl
	// over is the control under the pointer, and down the one held, -1
	// for none; hot and squash light and press each by its index.
	over, down  int
	hot, squash [16]float32
	// scrub is where a drag along the timeline is, in seconds, while it
	// drags, and hoverAt where the pointer is over it, -1 for neither.
	scrubbing bool
	scrub     float64
	hoverAt   float64
	lastSent  time.Time
}

// ctl is a control of the strip.
type ctl struct {
	id int
	r  geom.Rect
}

// The controls.
const (
	cPrev = iota
	cPlay
	cNext
	cRepeat
	cSlower
	cSpeed
	cFaster
	cDown
	cTranspose
	cUp
	cVolume
	cOpen
	cList
)

func (t *transport) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	w, h := c.Max.W, c.Max.H
	t.timeline = xyxy(14, 10, w-14, 40)
	y := float32(48)
	row := h - y - 6
	mid := y + row/2
	t.ctls = t.ctls[:0]
	add := func(id int, x, w, hh float32) float32 {
		t.ctls = append(t.ctls, ctl{id, xyxy(x, mid-hh/2, x+w, mid+hh/2)})
		return x + w
	}
	x := float32(14)
	x = add(cPrev, x, 40, 40) + 8
	x = add(cPlay, x, 52, 52) + 8
	x = add(cNext, x, 40, 40) + 10
	add(cRepeat, x, 34, 34)
	// From the right: the playlist, Open, the volume, the transpose and
	// the speed.
	rx := w - 14
	rx -= 40
	add(cList, rx, 40, 40)
	rx -= 48
	add(cOpen, rx, 40, 40)
	rx -= 140
	add(cVolume, rx, 124, 24)
	rx -= 150
	add(cDown, rx, 28, 28)
	add(cTranspose, rx+28, 72, 28)
	add(cUp, rx+100, 28, 28)
	rx -= 150
	add(cSlower, rx, 28, 28)
	add(cSpeed, rx+28, 72, 28)
	add(cFaster, rx+100, 28, 28)
	return c.Max
}

// ctlAt returns the index of the control at pos, -1 for none.
func (t *transport) ctlAt(pos geom.Point) int {
	for i, c := range t.ctls {
		if c.r.Inset(geom.Uniform(-3)).Contains(pos) {
			return i
		}
	}
	return -1
}

// timeAt returns the time in the song at x along the timeline.
func (t *transport) timeAt(x float32) float64 {
	sc := t.l.score
	if sc == nil {
		return 0
	}
	f := (x - t.timeline.Min.X) / t.timeline.Size().W
	return float64(min(max(f, 0), 1)) * sc.Length
}

func (t *transport) Handle(e input.Event, u *gunim.UI) bool {
	s := t.l.s
	switch e := e.(type) {
	case input.PointerMove:
		if t.scrubbing {
			t.scrub = t.timeAt(e.Pos.X)
			// While it drags, the song follows a few times a second.
			if time.Since(t.lastSent) > 120*time.Millisecond {
				t.lastSent = time.Now()
				u.Send(t, Sought{Time: t.scrub})
			}
			u.Invalidate()
			return true
		}
		if t.down == cVolume+100 {
			u.Send(t, VolumeSet{Volume: t.volumeAt(e.Pos.X)})
			return true
		}
		t.hoverAt = -1
		if t.timeline.Inset(geom.Uniform(-4)).Contains(e.Pos) && t.l.score != nil {
			t.hoverAt = t.timeAt(e.Pos.X)
		}
		t.over = t.ctlAt(e.Pos)
		u.Invalidate()
		return false
	case input.PointerLeave:
		t.over, t.hoverAt = -1, -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if t.timeline.Inset(geom.Uniform(-4)).Contains(e.Pos) && t.l.score != nil {
			t.scrubbing, t.scrub = true, t.timeAt(e.Pos.X)
			t.lastSent = time.Now()
			u.Send(t, Sought{Time: t.scrub})
			return true
		}
		i := t.ctlAt(e.Pos)
		if i < 0 {
			return false
		}
		t.down = i
		t.squash[i] = 1
		if t.ctls[i].id == cVolume {
			t.down = cVolume + 100
			u.Send(t, VolumeSet{Volume: t.volumeAt(e.Pos.X)})
		}
		return true
	case input.PointerUp:
		if t.scrubbing {
			t.scrubbing = false
			u.Send(t, Sought{Time: t.timeAt(e.Pos.X)})
			return true
		}
		if t.down == cVolume+100 {
			t.down = -1
			return true
		}
		i := t.ctlAt(e.Pos)
		if i >= 0 && i == t.down {
			t.act(t.ctls[i].id, s, u)
		}
		t.down = -1
	default:
		return false
	}
	return true
}

// act carries out control id.
func (t *transport) act(id int, s Player, u *gunim.UI) {
	switch id {
	case cPrev:
		u.Send(t, Skipped{By: -1})
	case cPlay:
		u.Send(t, PlayToggled{})
	case cNext:
		u.Send(t, Skipped{By: 1})
	case cRepeat:
		u.Send(t, RepeatToggled{})
	case cSlower:
		u.Send(t, SpeedSet{Speed: s.Speed - 0.1})
	case cSpeed:
		u.Send(t, SpeedSet{Speed: 1})
	case cFaster:
		u.Send(t, SpeedSet{Speed: s.Speed + 0.1})
	case cDown:
		u.Send(t, TransposeSet{Semis: s.Transpose - 1})
	case cTranspose:
		u.Send(t, TransposeSet{Semis: 0})
	case cUp:
		u.Send(t, TransposeSet{Semis: s.Transpose + 1})
	case cOpen:
		u.Send(t, OpenAsked{})
	case cList:
		t.list.open = !t.list.open
		t.list.closed = !t.list.open
		u.Invalidate()
	}
}

func (t *transport) volumeAt(x float32) float32 {
	for _, c := range t.ctls {
		if c.id == cVolume {
			return min(max((x-c.r.Min.X)/c.r.Size().W, 0), 1)
		}
	}
	return 1
}

func (t *transport) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := t.l
	pal := l.pal
	dt := float32(f.Delta.Seconds())
	for i := range t.hot {
		want := float32(0)
		if i == t.over {
			want = 1
		}
		t.hot[i] += (want - t.hot[i]) * min(1, dt*14)
		t.squash[i] = max(0, t.squash[i]-dt*5)
	}
	full := geom.Rect{Max: box.Point()}
	rad := 20 * pal.round
	p.ShadowRRect(full, rad, paint.Solid(alpha(pal.panel, 0.94)), paint.Shadow{Blur: 24, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.4)})
	p.RRectStroke(full, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.8)})
	t.paintTimeline(p, pal)
	for i, c := range t.ctls {
		t.paintCtl(p, i, c, pal)
	}
}

// paintTimeline draws the song as a map of how busy it is, each slice
// in the colour of the channel busiest then, played brighter than to
// come, with the playhead glowing where the song is heard.
func (t *transport) paintTimeline(p *paint.Painter, pal palette) {
	l := t.l
	r := t.timeline
	pn := pen{pal: pal}
	p.RRect(r, 6*pal.round, paint.Solid(alpha(pal.bgBottom, 0.5)))
	sc := l.score
	if sc == nil {
		msg := shaped("The timeline shows the song once one plays", 11, false)
		msg.Paint(p, geom.Pt(r.Min.X+12, r.Min.Y+(r.Size().H-msg.Height())/2), alpha(pal.dim, 0.7))
		return
	}
	at := l.t
	if t.scrubbing {
		at = t.scrub
	}
	head := r.Min.X + r.Size().W*float32(at/sc.Length)
	n := len(sc.Density)
	bw := r.Size().W / float32(n)
	mid := r.Min.Y + r.Size().H/2
	for i, d := range sc.Density {
		var sum, best float32
		top := 0
		for ch, v := range d {
			sum += v
			if v > best {
				best, top = v, ch
			}
		}
		if sum <= 0.001 {
			continue
		}
		x := r.Min.X + float32(i)*bw
		hh := max(2, sum*(r.Size().H-6)) / 2
		c := pal.ch[top]
		if x > head {
			c = alpha(mix(c, pal.dim, 0.5), 0.45)
		}
		// The slices grow up from the middle as the song loads.
		hh *= ease(l.fresh*1.5 - float32(i)/float32(n)*0.5)
		pn.rect(p, xyxy(x, mid-hh, x+max(bw-0.6, 0.6), mid+hh), 0, c)
	}
	// The playhead.
	func() {
		defer p.Blend(paint.BlendAdd)()
		p.ShadowRRect(xyxy(head-1.5, r.Min.Y-3, head+1.5, r.Max.Y+3), 1.5, paint.Solid(pal.accent),
			paint.Shadow{Blur: 10 + 10*l.pulse, Color: alpha(pal.accent, 0.8)})
	}()
	// The time, at the left of the controls, and where the pointer is.
	if t.hoverAt >= 0 && !t.scrubbing {
		x := r.Min.X + r.Size().W*float32(t.hoverAt/sc.Length)
		p.RRect(xyxy(x-0.5, r.Min.Y, x+0.5, r.Max.Y), 0, paint.Solid(alpha(pal.ink, 0.5)))
		tip := shaped(clock(t.hoverAt), 11, true)
		tx := min(max(x-tip.Advance/2, r.Min.X), r.Max.X-tip.Advance)
		tip.Paint(p, geom.Pt(tx, r.Min.Y-tip.Height()-1), pal.ink)
	}
}

// paintCtl draws control i.
func (t *transport) paintCtl(p *paint.Painter, i int, c ctl, pal palette) {
	s := t.l.s
	hot, sq := t.hot[i], t.squash[i]
	mid := geom.Pt(c.r.Min.X+c.r.Size().W/2, c.r.Min.Y+c.r.Size().H/2)
	end := p.Push(paint.Scale(1-0.08*sq+0.04*hot, mid))
	defer end()
	round := func(ic *icon.Icon, fill, ink color.NRGBA) {
		rr := c.r.Size().H / 2 * min(1, pal.round*3)
		p.ShadowRRect(c.r, rr, paint.Solid(lighten(fill, 0.1*hot)), paint.Shadow{Blur: 6 + 6*hot, Offset: geom.Pt(0, 2), Color: alpha(color.NRGBA{A: 255}, 0.35)})
		sz := c.r.Size().H * 0.45
		p.Mask(icon.Stroke{Icon: ic, Width: 2, Progress: 1}, xyxy(mid.X-sz/2, mid.Y-sz/2, mid.X+sz/2, mid.Y+sz/2), ink)
	}
	small := func(label string) {
		run := shaped(label, 16, true)
		rr := c.r.Size().H / 2 * min(1, pal.round*3)
		p.RRect(c.r, rr, paint.Solid(alpha(lighten(pal.panel, 0.08+0.12*hot), 1)))
		run.Paint(p, geom.Pt(mid.X-run.Advance/2, mid.Y-run.Height()/2), pal.ink)
	}
	readout := func(value, caption string, changed bool) {
		v := shaped(value, 14, true)
		ink := pal.ink
		if changed {
			ink = pal.accent
		}
		v.Paint(p, geom.Pt(mid.X-v.Advance/2, c.r.Min.Y-1), ink)
		cap := shaped(caption, 9, false)
		cap.Paint(p, geom.Pt(mid.X-cap.Advance/2, c.r.Max.Y-cap.Height()+2), alpha(pal.dim, 0.9))
	}
	switch c.id {
	case cPrev:
		round(icon.SkipBack, lighten(pal.panel, 0.08), pal.ink)
	case cNext:
		round(icon.SkipForward, lighten(pal.panel, 0.08), pal.ink)
	case cPlay:
		// The play button breathes with the beat while it plays.
		glow := pal.accent
		if s.Playing {
			defer func() {
				func() {
					defer p.Blend(paint.BlendAdd)()
					rr := c.r.Size().H / 2 * min(1, pal.round*3)
					p.ShadowRRect(c.r, rr, paint.Solid(color.NRGBA{}), paint.Shadow{Blur: 10 + 22*t.l.pulse, Color: alpha(glow, 0.5+0.4*t.l.pulse)})
				}()
			}()
		}
		ic := icon.Play
		if s.Playing {
			ic = icon.Pause
		}
		round(ic, pal.accent, darken(pal.accent, 0.8))
	case cRepeat:
		ic := icon.Repeat
		ink := alpha(pal.dim, 0.8)
		switch s.Repeat {
		case RepeatAll:
			ink = pal.accent
		case RepeatOne:
			ic, ink = icon.Repeat1, pal.accent
		}
		round(ic, alpha(pal.panel, 0), ink)
	case cOpen:
		round(icon.FolderOpen, lighten(pal.panel, 0.08), pal.ink)
	case cList:
		round(icon.ListMusic, lighten(pal.panel, 0.08), pal.ink)
	case cSlower, cDown:
		small("−")
	case cFaster, cUp:
		small("+")
	case cSpeed:
		readout(fmt.Sprintf("%.1f×", s.Speed), "SPEED", s.Speed < 0.95 || s.Speed > 1.05)
	case cTranspose:
		v := fmt.Sprintf("%+d", s.Transpose)
		if s.Transpose == 0 {
			v = "0"
		}
		readout(v, "TRANSPOSE", s.Transpose != 0)
	case cVolume:
		track := xyxy(c.r.Min.X, mid.Y-3, c.r.Max.X, mid.Y+3)
		p.RRect(track, 3*pal.round, paint.Solid(alpha(pal.edge, 0.6)))
		x := track.Min.X + track.Size().W*s.Volume
		p.RRect(xyxy(track.Min.X, track.Min.Y, x, track.Max.Y), 3*pal.round, paint.Solid(pal.accent))
		k := 8 + 2*hot
		p.ShadowRRect(xyxy(x-k, mid.Y-k, x+k, mid.Y+k), k*min(1, pal.round*3), paint.Solid(pal.ink), paint.Shadow{Blur: 6, Color: alpha(color.NRGBA{A: 255}, 0.4)})
		cap := shaped("VOLUME", 9, false)
		cap.Paint(p, geom.Pt(c.r.Min.X, c.r.Max.Y+2), alpha(pal.dim, 0.9))
	}
	// The time beside the repeat button.
	if c.id == cRepeat {
		sc := t.l.score
		now, total := "0:00", "0:00"
		if sc != nil {
			now, total = clock(t.l.t), clock(sc.Length)
		}
		run := shaped(now+" / "+total, 15, true)
		run.Paint(p, geom.Pt(c.r.Max.X+14, mid.Y-run.Height()/2), pal.ink)
	}
}
