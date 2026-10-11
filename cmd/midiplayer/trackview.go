package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"

	"github.com/marrasen/gunim-game-audio/tracker"
)

// trackView is a tracker's module as a tracker shows it, as it plays:
// the pattern scrolling under a bar of the row heard, each cell popping
// and throwing sparks as it plays; a scope and a meter for each channel
// over it; the order list and the instruments either side, each lit as
// it plays; and behind it all the copper bars and the stars of an Amiga
// demo, with a scroller of the sample names, where tracker musicians
// wrote their greetings, waving along the bottom.
type trackView struct {
	l    *look
	size geom.Size
	// origin is where the view lies in the window, for the sparks.
	origin geom.Point
	// meter is each channel's level as its meter shows it, falling, and
	// hold the peak each holds.
	meter, hold [maxCh]float32
	// insLit is how lit each instrument is, falling from 1 as it plays.
	insLit []float32
	// lastRow is the row the view last showed, to see a new one come.
	lastRow int
	// pop is how far each channel's cell in the row heard pops, falling
	// from 1.
	pop [maxCh]float32
	// orderY is where the order list is scrolled to, easing.
	orderY float32
	// scroll is the scroller's text, for the song it was made for.
	scroll    string
	scrollFor *tracker.Module
	// sparks fly from the cells played to their instruments.
	sparks []tspark
	// insOff is how far the instrument list is scrolled, gliding to
	// insWant; latest is the instrument last played, -1 for none;
	// manual counts down the seconds the wheel's scroll holds; insList
	// is where the list lies.
	insOff, insWant float32
	latest          int
	manual          float32
	insList         geom.Rect
	// shown are the instruments the list shows, those with a name or a
	// sample, and rowOf the row each instrument shows in, -1 for none;
	// shownFor is the module they are for.
	shown    []int
	rowOf    []int
	shownFor *tracker.Module
	// patFlash flashes as a new pattern comes in.
	patFlash float32
	lastPat  int
	t        float64
}

// tspark is a spark flying from a cell to its instrument.
type tspark struct {
	from, to geom.Point
	age      float32
	col      color.NRGBA
}

func (tv *trackView) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	tv.size = c.Max
	return c.Max
}

// mono is s shaped in the monospaced face, at size.
func mono(s string, size float32, bold bool) text.Run { return audioui.Shaped(s, size, bold, true) }

// The view's measures.
const (
	tvScopeH  = 92
	tvOrderW  = 112
	tvInsW    = 236
	tvScrollH = 54
	tvRowH    = 17
)

func (tv *trackView) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := tv.l
	sc := l.score
	if sc == nil || sc.Tracker == nil || box.W < 300 || box.H < 300 {
		return
	}
	ts := sc.Tracker
	m := ts.Module
	pal := l.pal
	dt := float32(f.Delta.Seconds())
	tv.t += float64(dt)
	tv.fall(dt, m)
	full := geom.Rect{Max: box.Point()}
	rad := 18 * pal.round
	p.ShadowRRect(full, rad, paint.Solid(alpha(darken(pal.bgBottom, 0.3), 0.97)),
		paint.Shadow{Blur: 22, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.45)})
	end := p.Layer(paint.LayerOpts{Bounds: full, Opacity: 1, Clip: true, Radius: rad})
	defer end()
	ri := ts.rowAt(l.t)
	row := tracker.RowAt{}
	frac := float32(0)
	if ri >= 0 {
		row = ts.Rows[ri]
		if ri+1 < len(ts.Rows) {
			next := ts.Rows[ri+1].Time
			frac = float32(min(max((l.t-row.Time)/max(next-row.Time, 1e-6), 0), 1))
		}
	}
	if ri != tv.lastRow {
		tv.newRow(ts, ri, box)
		tv.lastRow = ri
	}
	if ri >= 0 && row.Pattern != tv.lastPat {
		tv.lastPat, tv.patFlash = row.Pattern, 1
	}
	tv.paintStars(p, box)
	grid := xyxy(tvOrderW+12, tvScopeH+14, box.W-tvInsW-12, box.H-tvScrollH-8)
	tv.paintCopper(p, grid)
	tv.paintScopes(p, xyxy(10, 10, box.W-10, tvScopeH+4), m)
	tv.paintOrders(p, xyxy(10, tvScopeH+14, tvOrderW, box.H-tvScrollH-8), m, row, ri >= 0)
	tv.paintPattern(p, grid, ts, ri, frac)
	tv.insList = xyxy(box.W-tvInsW, tvScopeH+14, box.W-10, box.H-tvScrollH-8)
	tv.followIns(tv.insList, dt)
	tv.paintInstruments(p, tv.insList, m)
	tv.paintSparks(p, dt)
	tv.paintScroller(p, xyxy(0, box.H-tvScrollH, box.W, box.H), m)
	tv.paintCounters(p, xyxy(tvOrderW+12, box.H-tvScrollH-8-0, box.W-tvInsW-12, box.H-tvScrollH-8), m, row)
}

// fall lets the meters, the instruments' lights and the pops fall, and
// takes the meters' latest readings.
func (tv *trackView) fall(dt float32, m *tracker.Module) {
	mt := tv.l.s.Meters
	k := float32(math.Pow(10, float64(-dt*30/20)))
	hk := float32(math.Pow(10, float64(-dt*8/20)))
	for i := range tv.meter {
		tv.meter[i] = max(tv.meter[i]*k, mt.Channels[i]*1.6)
		tv.hold[i] = max(tv.hold[i]*hk, tv.meter[i])
		tv.pop[i] = max(0, tv.pop[i]-dt*3.5)
	}
	if len(tv.insLit) != len(m.Instruments) {
		tv.insLit = make([]float32, len(m.Instruments))
	}
	if tv.shownFor != m {
		tv.shownFor = m
		tv.shown, tv.rowOf = tv.shown[:0], make([]int, len(m.Instruments))
		for i, in := range m.Instruments {
			tv.rowOf[i] = -1
			if strings.TrimSpace(in.Name) != "" || hasSound(in) {
				tv.rowOf[i] = len(tv.shown)
				tv.shown = append(tv.shown, i)
			}
		}
	}
	for i := range tv.insLit {
		tv.insLit[i] = max(0, tv.insLit[i]-dt*2.2)
	}
	tv.patFlash = max(0, tv.patFlash-dt*2.5)
}

// newRow sees a new row come: its notes pop, light their instruments,
// and send sparks flying to them.
func (tv *trackView) newRow(ts *TrackerSong, ri int, box geom.Size) {
	if ri < 0 || ri >= len(ts.Rows) || !tv.l.playing {
		return
	}
	m := ts.Module
	r := ts.Rows[ri]
	pat := &m.Patterns[min(r.Pattern, len(m.Patterns)-1)]
	if r.Row >= pat.Rows {
		return
	}
	grid := xyxy(tvOrderW+12, tvScopeH+14, box.W-tvInsW-12, box.H-tvScrollH-8)
	cw := tv.colW(grid, m.Channels)
	for ch := range m.Channels {
		c := pat.Cell(r.Row, ch, m.Channels)
		if c.Note == 0 || c.Note == tracker.KeyOff {
			if c.Effect != 0 || c.Param != 0 {
				tv.pop[ch] = max(tv.pop[ch], 0.4)
			}
			continue
		}
		tv.pop[ch] = 1
		ins := int(c.Instrument)
		if ins > 0 && ins <= len(tv.insLit) {
			tv.insLit[ins-1] = 1
			tv.latest = ins - 1
			if len(tv.sparks) < 80 {
				y := grid.Min.Y + grid.Size().H*0.42
				from := geom.Pt(grid.Min.X+34+cw*(float32(ch)+0.5), y)
				if to, ok := tv.insAt(box, ins-1); ok {
					tv.sparks = append(tv.sparks, tspark{from: from, to: to, col: tv.l.pal.ch[ch]})
				}
			}
		}
	}
}

// colW returns how wide each channel's column of the pattern is.
func (tv *trackView) colW(grid geom.Rect, channels int) float32 {
	return (grid.Size().W - 34) / float32(max(channels, 1))
}

// insAt returns where instrument i's row is in the list, in the view.
func (tv *trackView) insAt(box geom.Size, i int) (geom.Point, bool) {
	if i < 0 || i >= len(tv.rowOf) || tv.rowOf[i] < 0 {
		return geom.Point{}, false
	}
	list := xyxy(box.W-tvInsW, tvScopeH+14, box.W-10, box.H-tvScrollH-8)
	y := list.Min.Y + 30 + float32(tv.rowOf[i])*tv.insRowH(list) - tv.insScroll(list)
	return geom.Pt(list.Min.X+16, y+tv.insRowH(list)/2), true
}

// hasSound reports whether an instrument has a sample with any sound.
func hasSound(in tracker.Instrument) bool {
	for _, s := range in.Samples {
		if len(s.Data) > 2 {
			return true
		}
	}
	return false
}

func (tv *trackView) insRowH(list geom.Rect) float32 {
	n := max(len(tv.shown), 1)
	return min(max((list.Size().H-36)/float32(n), 15), 22)
}

// insScroll returns how far the instrument list is scrolled.
func (tv *trackView) insScroll(list geom.Rect) float32 {
	return min(max(tv.insOff, 0), tv.insMost(list))
}

// insMost returns how far the instrument list can scroll: none where it
// fits.
func (tv *trackView) insMost(list geom.Rect) float32 {
	return max(0, float32(len(tv.shown))*tv.insRowH(list)-(list.Size().H-36))
}

// followIns scrolls the instrument list, where it does not fit, to the
// instrument last played, and only once that is out of view, gliding
// there; for a while after the wheel scrolls it, it stays where the
// wheel left it.
func (tv *trackView) followIns(list geom.Rect, dt float32) {
	most := tv.insMost(list)
	if most <= 0 {
		tv.insOff, tv.insWant = 0, 0
		return
	}
	h := tv.insRowH(list)
	room := list.Size().H - 36
	if tv.manual > 0 {
		tv.manual -= dt
	} else if tv.latest >= 0 && tv.latest < len(tv.rowOf) && tv.rowOf[tv.latest] >= 0 {
		top := float32(tv.rowOf[tv.latest]) * h
		switch {
		case top < tv.insWant:
			tv.insWant = top - h
		case top+h > tv.insWant+room:
			tv.insWant = top + 2*h - room
		}
	}
	tv.insWant = min(max(tv.insWant, 0), most)
	tv.insOff += (tv.insWant - tv.insOff) * min(1, dt*6)
	if math.Abs(float64(tv.insWant-tv.insOff)) < 0.5 {
		tv.insOff = tv.insWant
	}
}

// Handle scrolls the instrument list with the wheel.
func (tv *trackView) Handle(e input.Event, u *gunim.UI) bool {
	s, ok := e.(input.Scroll)
	if !ok || !tv.insList.Contains(s.Pos) || tv.insMost(tv.insList) <= 0 {
		return false
	}
	tv.insWant = min(max(tv.insWant-s.Delta.Y, 0), tv.insMost(tv.insList))
	tv.manual = 4
	u.Invalidate()
	return true
}

// paintStars draws stars flying out from the middle, faster as the song
// is louder.
func (tv *trackView) paintStars(p *paint.Painter, box geom.Size) {
	pal := tv.l.pal
	var loud float32
	for _, v := range tv.meter {
		loud += v
	}
	cx, cy := box.W/2, box.H*0.45
	defer p.Blend(paint.BlendAdd)()
	for i := range 90 {
		seed := float64(i)*7.31 + 1
		a := math.Mod(seed*2.399, 2*math.Pi)
		speed := 0.08 + 0.12*math.Mod(seed*0.618, 1)
		z := math.Mod(tv.t*speed*(1+float64(min(loud, 3))*0.8)+math.Mod(seed*0.37, 1), 1)
		d := float32(z*z) * max(box.W, box.H) * 0.7
		x, y := cx+cosf(a)*d, cy+sinf(a)*d
		s := 1 + 2.5*float32(z)
		c := alpha(mix(pal.white, pal.ch[i%maxCh], 0.4), float32(z)*0.8)
		pen{pal: pal}.rect(p, xyxy(x-s/2, y-s/2, x+s/2, y+s/2), s/2, c)
	}
}

// paintCopper draws an Amiga's copper bars sweeping up and down behind
// the pattern, brighter on the beat.
func (tv *trackView) paintCopper(p *paint.Painter, r geom.Rect) {
	pal := tv.l.pal
	defer p.Blend(paint.BlendAdd)()
	for i := range 5 {
		ph := tv.t*1.3 + float64(i)*0.55
		y := r.Min.Y + r.Size().H*(0.5+0.42*sinf(ph))
		h := float32(26)
		c := pal.ch[(i*5+2)%maxCh]
		a := 0.16 + 0.22*tv.l.pulse
		p.RRect(xyxy(r.Min.X, y-h/2, r.Max.X, y+h/2), 0, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, y-h/2),
			To: geom.Pt(0, y+h/2), Start: alpha(c, 0), End: alpha(c, 0),
			Stops: []paint.Stop{{At: 0.5, Color: alpha(lighten(c, 0.3), a)}}}})
	}
}

// paintScopes draws a scope and a meter for each channel: the wave of the
// sample it plays, at the pitch it plays it, as loud as the channel is.
func (tv *trackView) paintScopes(p *paint.Painter, r geom.Rect, m *tracker.Module) {
	l := tv.l
	pal := l.pal
	pn := pen{pal: pal}
	n := m.Channels
	gap := float32(6)
	if n > 16 {
		gap = 3
	}
	w := (r.Size().W - gap*float32(n-1)) / float32(n)
	// The note each channel plays now.
	var now [maxCh]*Note
	l.score.span(l.t-3, l.t, func(nt *Note) {
		if nt.Start <= l.t && int(nt.Ch) < maxCh && (now[nt.Ch] == nil || nt.Start > now[nt.Ch].Start) {
			now[nt.Ch] = nt
		}
	})
	for ch := range n {
		x := r.Min.X + float32(ch)*(w+gap)
		box := xyxy(x, r.Min.Y, x+w, r.Max.Y)
		col := pal.ch[ch]
		muted := l.s.Muted[ch] || l.s.Solo >= 0 && l.s.Solo != ch
		lit := min(tv.meter[ch], 1)
		p.RRect(box, 8*pal.round, paint.Fill{Gradient: &paint.Gradient{From: box.Min, To: geom.Pt(box.Min.X, box.Max.Y),
			Start: alpha(darken(col, 0.85), 0.95), End: alpha(darken(col, 0.7-0.3*lit), 0.95)}})
		p.RRectStroke(box, 8*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(col, 0.3+0.5*lit)})
		// The meter, up the left edge.
		mh := box.Size().H - 8
		vu := xyxy(box.Min.X+4, box.Min.Y+4, box.Min.X+8, box.Max.Y-4)
		pn.rect(p, vu, 2, alpha(pal.bgBottom, 0.7))
		top := vu.Max.Y - mh*min(tv.meter[ch], 1)
		pn.rect(p, xyxy(vu.Min.X, top, vu.Max.X, vu.Max.Y), 2, col)
		hy := vu.Max.Y - mh*min(tv.hold[ch], 1)
		pn.rect(p, xyxy(vu.Min.X-1, hy-1, vu.Max.X+1, hy+1), 0, pal.white)
		// The scope.
		sr := xyxy(box.Min.X+12, box.Min.Y+16, box.Max.X-4, box.Max.Y-4)
		mid := (sr.Min.Y + sr.Max.Y) / 2
		pn.rect(p, xyxy(sr.Min.X, mid, sr.Max.X, mid+1), 0, alpha(col, 0.2))
		label := fmt.Sprintf("%02d", ch+1)
		if w > 60 {
			if nt := now[ch]; nt != nil && int(nt.Ins) >= 1 && int(nt.Ins) <= len(m.Instruments) {
				label += " " + strings.TrimSpace(m.Instruments[nt.Ins-1].Name)
			}
		}
		fit(label, 9, true, box.Size().W-16).Paint(p, geom.Pt(box.Min.X+12, box.Min.Y+3), alpha(pal.ink, 0.8))
		if muted {
			mt := mono("MUTE", 9, true)
			mt.Paint(p, geom.Pt(box.Max.X-mt.Advance-4, box.Min.Y+3), rgb(0xff6b6b))
			continue
		}
		nt := now[ch]
		if nt == nil || lit < 0.01 {
			continue
		}
		smp := sampleOf(m, nt)
		if smp == nil || len(smp.Data) == 0 {
			continue
		}
		// Where in the sample the channel is, and a few cycles from there.
		pos := (l.t - nt.Start) * nt.Freq
		amp := min(lit, 1) * sr.Size().H * 0.55
		const pts = 48
		span := 0.012 * nt.Freq
		prev := geom.Point{}
		for k := 0; k <= pts; k++ {
			v := sampleAt(smp, pos+span*float64(k)/pts)
			pt := geom.Pt(sr.Min.X+sr.Size().W*float32(k)/pts, mid-v*amp)
			if k > 0 {
				pn.line(p, prev, pt, 1.6, lighten(col, 0.35))
			}
			prev = pt
		}
		// A faint glow along the trace, as a scope's phosphor has.
		func() {
			defer p.Blend(paint.BlendAdd)()
			p.RRect(xyxy(sr.Min.X, mid-amp, sr.Max.X, mid+amp), 0, paint.Fill{Gradient: &paint.Gradient{
				From: geom.Pt(0, mid-amp), To: geom.Pt(0, mid+amp), Start: alpha(col, 0), End: alpha(col, 0),
				Stops: []paint.Stop{{At: 0.5, Color: alpha(col, 0.18*lit)}}}})
		}()
	}
}

// sampleOf returns the sample a note plays.
func sampleOf(m *tracker.Module, nt *Note) *tracker.Sample {
	if nt.Ins < 1 || int(nt.Ins) > len(m.Instruments) {
		return nil
	}
	in := &m.Instruments[nt.Ins-1]
	if len(in.Samples) == 0 {
		return nil
	}
	i := 0
	if k := int(nt.Key) - 12; k >= 0 && k < 96 {
		i = int(in.Keymap[k])
	}
	if i >= len(in.Samples) {
		i = 0
	}
	return &in.Samples[i]
}

// sampleAt returns sample s's sound at pos frames into it, round its
// loop, or silence past an end it does not loop at.
func sampleAt(s *tracker.Sample, pos float64) float32 {
	n := len(s.Data)
	ip := int(pos)
	if s.Loop != 0 && s.LoopLen > 1 && ip >= s.LoopStart+s.LoopLen {
		ip = s.LoopStart + (ip-s.LoopStart)%s.LoopLen
	}
	if ip < 0 || ip >= n {
		return 0
	}
	return s.Data[ip]
}

// paintOrders draws the order list, scrolling to keep the order heard in
// the middle, its pattern's number lit.
func (tv *trackView) paintOrders(p *paint.Painter, r geom.Rect, m *tracker.Module, row tracker.RowAt, playing bool) {
	pal := tv.l.pal
	pn := pen{pal: pal}
	p.RRect(r, 10*pal.round, paint.Solid(alpha(pal.panel, 0.88)))
	p.RRectStroke(r, 10*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	mono("ORDERS", 10, true).Paint(p, geom.Pt(r.Min.X+10, r.Min.Y+8), pal.dim)
	rh := float32(20)
	want := float32(row.Order) * rh
	tv.orderY += (want - tv.orderY) * 0.18
	list := xyxy(r.Min.X, r.Min.Y+26, r.Max.X, r.Max.Y-4)
	end := p.Layer(paint.LayerOpts{Bounds: list, Opacity: 1, Clip: true, Fade: geom.Insets{Top: 20, Bottom: 20}})
	defer end()
	mid := list.Min.Y + list.Size().H/2
	for i, pat := range m.Orders {
		y := mid + float32(i)*rh - tv.orderY - rh/2
		if y < list.Min.Y-rh || y > list.Max.Y {
			continue
		}
		cur := playing && i == row.Order
		ink := alpha(pal.ink, 0.55)
		if cur {
			pn.rect(p, xyxy(list.Min.X+6, y, list.Max.X-6, y+rh-2), 4, alpha(pal.accent, 0.25+0.2*tv.patFlash))
			ink = pal.accent
		}
		mono(fmt.Sprintf("%02X", i), 11, false).Paint(p, geom.Pt(list.Min.X+12, y+3), alpha(pal.dim, 0.8))
		mono(fmt.Sprintf("%02d", pat), 12, cur).Paint(p, geom.Pt(list.Min.X+44, y+2), ink)
		if cur {
			mono("◀", 10, true).Paint(p, geom.Pt(list.Max.X-22, y+3), pal.accent)
		}
	}
}

// paintPattern draws the pattern as it plays: the rows heard and to
// come, scrolling smoothly up through a bar at the row heard.
func (tv *trackView) paintPattern(p *paint.Painter, r geom.Rect, ts *TrackerSong, ri int, frac float32) {
	l := tv.l
	pal := l.pal
	pn := pen{pal: pal}
	m := ts.Module
	p.RRect(r, 10*pal.round, paint.Solid(alpha(pal.bgBottom, 0.72)))
	p.RRectStroke(r, 10*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 10 * pal.round, Fade: geom.Insets{Top: 26, Bottom: 26}})
	defer end()
	cw := tv.colW(r, m.Channels)
	// How much of a cell fits: all of it, the note and instrument, or the
	// note alone.
	size := float32(12)
	detail := 3
	full := 13.0
	if m.Format == "MOD" {
		full = 10
	}
	switch {
	case cw >= float32(full)*7.3+8:
		detail = 3
	case cw >= 6*7.3+6:
		detail = 2
	case cw >= 3*7.3+4:
		detail = 1
	default:
		detail, size = 1, 9
	}
	barY := r.Min.Y + r.Size().H*0.42
	hex := m.Format == "XM"
	// The bar of the row heard, glowing on the beat.
	func() {
		defer p.Blend(paint.BlendAdd)()
		bc := pal.accent
		p.ShadowRRect(xyxy(r.Min.X+2, barY-1, r.Max.X-2, barY+tvRowH+1), 3, paint.Solid(alpha(bc, 0.16+0.14*l.pulse)),
			paint.Shadow{Blur: 14 + 14*l.pulse, Color: alpha(bc, 0.3+0.2*l.pulse)})
	}()
	// The channels' column lines, lit by their meters.
	for ch := 0; ch <= m.Channels; ch++ {
		x := r.Min.X + 34 + float32(ch)*cw
		a := float32(0.25)
		if ch < m.Channels {
			a += 0.4 * min(tv.meter[ch], 1)
		}
		c := pal.edge
		if ch < m.Channels {
			c = mix(pal.edge, pal.ch[ch], min(tv.meter[ch], 1))
		}
		pn.rect(p, xyxy(x-1, r.Min.Y, x, r.Max.Y), 0, alpha(c, a))
	}
	if ri < 0 {
		return
	}
	visible := int(r.Size().H/tvRowH) + 2
	from := ri - int(float32(visible)*0.42) - 1
	for i := from; i <= from+visible+1; i++ {
		y := barY + (float32(i-ri)-frac)*tvRowH
		if y < r.Min.Y-tvRowH || y > r.Max.Y {
			continue
		}
		if i < 0 || i >= len(ts.Rows) {
			continue
		}
		rw := ts.Rows[i]
		pat := &m.Patterns[min(rw.Pattern, len(m.Patterns)-1)]
		if rw.Row >= pat.Rows {
			continue
		}
		cur := i == ri
		// A beat's row, and a bar's, lighter.
		switch {
		case rw.Row%16 == 0:
			pn.rect(p, xyxy(r.Min.X, y, r.Max.X, y+tvRowH), 0, alpha(pal.edge, 0.22))
		case rw.Row%4 == 0:
			pn.rect(p, xyxy(r.Min.X, y, r.Max.X, y+tvRowH), 0, alpha(pal.edge, 0.1))
		}
		num := fmt.Sprintf("%02d", rw.Row)
		if hex {
			num = fmt.Sprintf("%02X", rw.Row)
		}
		nc := alpha(pal.dim, 0.75)
		if cur {
			nc = pal.accent
		}
		mono(num, 11, cur).Paint(p, geom.Pt(r.Min.X+8, y+2), nc)
		for ch := range m.Channels {
			c := pat.Cell(rw.Row, ch, m.Channels)
			x := r.Min.X + 34 + float32(ch)*cw + 5
			tv.paintCell(p, c, geom.Pt(x, y), cur, ch, detail, size, m)
		}
	}
}

// paintCell draws a cell at at: its note, instrument, volume and effect
// as fit, each in its own colour; in the row heard, a cell that plays
// pops and glows.
func (tv *trackView) paintCell(p *paint.Painter, c tracker.Cell, at geom.Point, cur bool, ch, detail int, size float32, m *tracker.Module) {
	pal := tv.l.pal
	col := pal.ch[ch]
	empty := alpha(pal.dim, 0.3)
	scale := float32(1)
	if cur {
		scale += 0.25 * ease(tv.pop[ch])
	}
	cw := size * 0.61
	endScale := func() {}
	if scale != 1 {
		endScale = p.Push(paint.Scale(scale, geom.Pt(at.X+cw*1.5, at.Y+tvRowH/2)))
	}
	defer endScale()
	if cur && c.Note != 0 && tv.pop[ch] > 0.05 {
		pen{pal: pal}.glow(p, at.X+cw*1.5, at.Y+tvRowH/2, 30*tv.pop[ch], alpha(col, 0.8))
	}
	x := at.X
	put := func(s string, c color.NRGBA, bold bool) {
		mono(s, size, bold).Paint(p, geom.Pt(x, at.Y+2), c)
		x += cw * float32(len(s)+1)
	}
	note := tracker.NoteName(c.Note, m.OctaveShift)
	nc := lighten(col, 0.25)
	switch {
	case c.Note == 0:
		nc = empty
	case c.Note == tracker.KeyOff:
		nc = alpha(pal.ink, 0.6)
	case cur:
		nc = lighten(col, 0.6)
	}
	put(note, nc, cur && c.Note != 0)
	if detail >= 2 {
		ins := ".."
		ic := empty
		if c.Instrument > 0 {
			ins, ic = fmt.Sprintf("%02X", c.Instrument), alpha(pal.ink, 0.85)
			if m.Format == "MOD" {
				ins = fmt.Sprintf("%02d", c.Instrument)
			}
		}
		put(ins, ic, false)
	}
	if detail >= 3 {
		if m.Format == "XM" {
			vol, vc := "..", empty
			if c.Volume >= 0x10 {
				vol, vc = volText(c.Volume), mix(pal.accent2, pal.ink, 0.3)
			}
			put(vol, vc, false)
		}
		fx, fc := "...", empty
		if c.Effect != 0 || c.Param != 0 {
			fx, fc = effectText(c.Effect, c.Param), pal.accent
		}
		put(fx, fc, false)
	}
}

// volText writes an XM's volume column as FastTracker shows it.
func volText(v uint8) string {
	if v >= 0x10 && v <= 0x50 {
		return fmt.Sprintf("%02X", v-0x10)
	}
	return string("-+DUSVPLRM"[min(max(int(v>>4)-6, 0), 9)]) + fmt.Sprintf("%X", v&15)
}

// effectText writes an effect as a tracker shows it: its letter or
// digit, and its parameter in hexadecimal.
func effectText(e, param uint8) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return fmt.Sprintf("%c%02X", letters[min(int(e), len(letters)-1)], param)
}

// paintInstruments draws the instrument list: each instrument's number,
// name and sample's wave, lit as it plays.
func (tv *trackView) paintInstruments(p *paint.Painter, r geom.Rect, m *tracker.Module) {
	pal := tv.l.pal
	pn := pen{pal: pal}
	p.RRect(r, 10*pal.round, paint.Solid(alpha(pal.panel, 0.88)))
	p.RRectStroke(r, 10*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	mono("INSTRUMENTS", 10, true).Paint(p, geom.Pt(r.Min.X+10, r.Min.Y+8), pal.dim)
	list := xyxy(r.Min.X, r.Min.Y+26, r.Max.X, r.Max.Y-4)
	end := p.Layer(paint.LayerOpts{Bounds: list, Opacity: 1, Clip: true, Fade: geom.Insets{Bottom: 16}})
	defer end()
	h := tv.insRowH(r)
	off := tv.insScroll(r)
	for row, i := range tv.shown {
		in := m.Instruments[i]
		y := list.Min.Y + 4 + float32(row)*h - off
		if y < list.Min.Y-h || y > list.Max.Y {
			continue
		}
		lit := float32(0)
		if i < len(tv.insLit) {
			lit = tv.insLit[i]
		}
		if lit > 0.01 {
			pn.rect(p, xyxy(list.Min.X+4, y, list.Max.X-4, y+h-1), 4, alpha(pal.accent, 0.35*lit))
		}
		num := mono(fmt.Sprintf("%02X", i+1), 10, lit > 0.5)
		num.Paint(p, geom.Pt(list.Min.X+8+3*lit, y+(h-num.Height())/2), mix(pal.dim, pal.accent, lit))
		name := strings.TrimSpace(in.Name)
		fit(name, 10, lit > 0.5, list.Size().W-100).Paint(p, geom.Pt(list.Min.X+30+3*lit, y+(h-12)/2), mix(alpha(pal.ink, 0.75), pal.white, lit))
		// The sample's wave, small, at the right.
		if len(in.Samples) > 0 && len(in.Samples[0].Data) > 0 {
			d := in.Samples[0].Data
			wx := list.Max.X - 62
			mid := y + h/2
			for k := range 28 {
				v := d[k*len(d)/28]
				hh := max(abs32f(v)*(h-4), 0.6)
				pn.rect(p, xyxy(wx+float32(k)*2, mid-hh/2, wx+float32(k)*2+1.2, mid+hh/2), 0, alpha(mix(pal.dim, pal.accent, lit), 0.8))
			}
		}
	}
}

func abs32f(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// paintSparks moves and draws the sparks flying to the instruments.
func (tv *trackView) paintSparks(p *paint.Painter, dt float32) {
	pal := tv.l.pal
	kept := tv.sparks[:0]
	defer p.Blend(paint.BlendAdd)()
	for _, s := range tv.sparks {
		s.age += dt * 1.6
		if s.age >= 1 {
			continue
		}
		e := ease(s.age)
		pos := geom.Pt(s.from.X+(s.to.X-s.from.X)*e, s.from.Y+(s.to.Y-s.from.Y)*e-60*sinf(float64(s.age)*math.Pi))
		sz := 4 + 3*(1-s.age)
		pen{pal: pal}.glow(p, pos.X, pos.Y, sz*4, alpha(s.col, 0.9*(1-s.age)))
		pen{pal: pal}.circle(p, pos.X, pos.Y, sz/2, alpha(lighten(s.col, 0.5), 1-s.age))
		kept = append(kept, s)
	}
	tv.sparks = kept
}

// paintScroller waves the song's name and its samples' names along the
// bottom, a letter at a time on a sine, in the colours of the channels.
func (tv *trackView) paintScroller(p *paint.Painter, r geom.Rect, m *tracker.Module) {
	pal := tv.l.pal
	if tv.scrollFor != m {
		tv.scrollFor = m
		var parts []string
		parts = append(parts, strings.ToUpper(strings.TrimSpace(m.Title)))
		for _, in := range m.Instruments {
			if n := strings.TrimSpace(in.Name); n != "" {
				parts = append(parts, n)
			}
		}
		tv.scroll = "***  " + strings.Join(parts, "  ·  ") + "  ***          "
	}
	rs := []rune(tv.scroll)
	if len(rs) == 0 {
		return
	}
	const size = 22
	adv := float32(size * 0.62)
	total := adv * float32(len(rs))
	off := float32(math.Mod(tv.t*110, float64(total)))
	mid := r.Min.Y + r.Size().H/2 - 10
	for i := 0; ; i++ {
		x := r.Min.X - off + float32(i)*adv
		if x > r.Max.X {
			break
		}
		if x < r.Min.X-adv {
			continue
		}
		ch := rs[i%len(rs)]
		if ch == ' ' {
			continue
		}
		y := mid + 9*sinf(float64(x)*0.025+tv.t*3.2)
		c := pal.ch[(i/2)%maxCh]
		mono(string(ch), size, true).Paint(p, geom.Pt(x, y), lighten(c, 0.2))
	}
}

// paintCounters draws the song's place and pace along the pattern's
// foot: its order, pattern, row, speed and BPM, and the tracker it
// comes from.
func (tv *trackView) paintCounters(p *paint.Painter, r geom.Rect, m *tracker.Module, row tracker.RowAt) {
	pal := tv.l.pal
	fields := []struct{ k, v string }{
		{"POS", fmt.Sprintf("%02X/%02X", row.Order, max(len(m.Orders)-1, 0))},
		{"PAT", fmt.Sprintf("%02d", row.Pattern)},
		{"ROW", fmt.Sprintf("%02d", row.Row)},
		{"SPD", fmt.Sprintf("%d", row.Speed)},
		{"BPM", fmt.Sprintf("%d", row.BPM)},
		{"CH", fmt.Sprintf("%d", m.Channels)},
	}
	x := r.Min.X + 8
	y := r.Min.Y - 22
	for _, f := range fields {
		k := mono(f.k, 9, true)
		v := mono(f.v, 12, true)
		w := k.Advance + v.Advance + 18
		pen{pal: pal}.rect(p, xyxy(x, y, x+w, y+18), 5, alpha(pal.panel, 0.9))
		k.Paint(p, geom.Pt(x+6, y+4), pal.dim)
		v.Paint(p, geom.Pt(x+k.Advance+12, y+2), pal.accent)
		x += w + 6
	}
	name := "FASTTRACKER II"
	if m.Format == "MOD" {
		name = "PROTRACKER"
		if m.Tracker == "Soundtracker" {
			name = "SOUNDTRACKER"
		}
	}
	logo := mono(name, 13, true)
	lx := r.Max.X - logo.Advance - 10
	bounce := 2 * sinf(tv.t*4)
	logo.Paint(p, geom.Pt(lx+1, y+2+bounce+1), alpha(color.NRGBA{A: 255}, 0.6))
	logo.Paint(p, geom.Pt(lx, y+2+bounce), mix(pal.accent, pal.white, 0.5+0.5*tv.l.pulse))
}
