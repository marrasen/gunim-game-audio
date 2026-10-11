package main

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"

	"github.com/marrasen/gunim-game-audio/synth"
)

// mixer is the mixing desk: a strip for each channel that plays, with
// its equaliser, drive, sends, pan, mute, solo and fader beside its
// meter; and the master, its equaliser drawn as a curve over the sound
// heard, with a handle for each band, the reverb, delay and chorus the
// strips send to, the compressor and the limiter, each showing how far
// it turns the sound down, and the master fader.
//
// A knob or a fader drags up and down, finer with Shift; the wheel
// turns it, and a double click sets it back. An equaliser's handle
// drags across for its frequency and up and down for its gain, and the
// wheel narrows or widens it. The wheel over the strips, with Shift,
// scrolls them.
type mixer struct {
	l    *look
	size geom.Size
	// local is the mix as the desk shows it: the application's, but for
	// a moment after an edit, while the edit is on its way.
	local    synth.GMMix
	haveMix  bool
	lastEdit time.Time
	ctls     []mctl
	// hover is the control under the pointer and drag the one held, -1
	// for none; dragY and dragX are where the drag is, and from the
	// value it started at.
	hover, drag  int
	dragX, dragY float32
	from         float64
	lastClick    time.Time
	lastClicked  int
	// scroll is how far the strips are scrolled, stripsW how wide their
	// room is, and stripsTotal how wide they all are.
	scroll, stripsW, stripsTotal float32
	// The meters as they fall, and the peaks they hold.
	ch, hold          [16]float32
	left, right, comp float32
	limit, lHold      float32
	rHold             float32
	spec              []float32
	eqRect            geom.Rect
	// strips are where each strip lies, and stripCh its channel, and
	// faders where its fader is.
	strips  []geom.Rect
	stripCh []int
	faders  []geom.Rect
}

// The kinds of control.
const (
	kKnob = iota
	kFader
	kToggle
	kButton
	kEQ
)

// mctl is a control of the desk: what it is, where, and how it reads
// and sets the mix.
type mctl struct {
	kind        int
	r           geom.Rect
	label       string
	lo, hi, def float64
	bipolar     bool
	get         func(m *synth.GMMix) float64
	set         func(m *synth.GMMix, v float64)
	format      func(v float64) string
	col         color.NRGBA
	// ch is the channel a mute or solo is for, and band the equaliser's
	// band a handle is.
	ch, band int
	act      func(u *gunim.UI, n gunim.Node)
}

func dbText(v float64) string {
	if v <= -59.5 {
		return "−∞"
	}
	return fmt.Sprintf("%+.1f", v)
}

func pctText(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

func panText(v float64) string {
	switch {
	case math.Abs(v) < 0.02:
		return "C"
	case v < 0:
		return fmt.Sprintf("L%.0f", -v*100)
	}
	return fmt.Sprintf("R%.0f", v*100)
}

func hzText(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", v/1000)
	}
	return fmt.Sprintf("%.0f", v)
}

// The master's measures.
const (
	masterMinW = 470
	stripMinW  = 92
)

func (m *mixer) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	m.size = c.Max
	if !m.haveMix || (m.drag < 0 && time.Since(m.lastEdit) > 600*time.Millisecond) {
		m.local, m.haveMix = m.l.s.Mix, true
	}
	m.layout()
	return c.Max
}

// layout puts every control in place.
func (m *mixer) layout() {
	m.ctls = m.ctls[:0]
	m.strips, m.stripCh, m.faders = m.strips[:0], m.stripCh[:0], m.faders[:0]
	w, h := m.size.W, m.size.H
	if w < 200 || h < 200 {
		return
	}
	pal := m.l.pal
	mw := min(max(w*0.42, masterMinW), 600)
	m.stripsW = w - mw - 14
	chs := []int{}
	if sc := m.l.score; sc != nil {
		chs = sc.used()
	}
	if len(chs) == 0 {
		for i := range 16 {
			chs = append(chs, i)
		}
	}
	sw := max(float32(stripMinW), min(118, m.stripsW/float32(len(chs))))
	m.stripsTotal = sw * float32(len(chs))
	m.scroll = min(max(m.scroll, 0), max(0, m.stripsTotal-m.stripsW))
	for i, ch := range chs {
		x := float32(i)*sw - m.scroll
		m.layoutStrip(ch, xyxy(x+3, 0, x+sw-3, h), pal.ch[ch])
	}
	m.layoutMaster(xyxy(w-mw, 0, w, h))
}

// layoutStrip puts channel ch's strip's controls in r.
func (m *mixer) layoutStrip(ch int, r geom.Rect, col color.NRGBA) {
	strip := func(mm *synth.GMMix) *synth.GMStrip { return &mm.Channels[ch] }
	ks := min(float32(30), (r.Size().W-24)/2)
	cell := ks + 22
	type knobDef struct {
		label       string
		lo, hi, def float64
		bipolar     bool
		get         func(s *synth.GMStrip) *float64
		format      func(float64) string
	}
	defs := []knobDef{
		{"HI", -15, 15, 0, true, func(s *synth.GMStrip) *float64 { return &s.High }, dbText},
		{"MID", -15, 15, 0, true, func(s *synth.GMStrip) *float64 { return &s.Mid }, dbText},
		{"LO", -15, 15, 0, true, func(s *synth.GMStrip) *float64 { return &s.Low }, dbText},
		{"DRIVE", 0, 1, 0, false, func(s *synth.GMStrip) *float64 { return &s.Drive }, pctText},
		{"REV", 0, 2, 1, false, func(s *synth.GMStrip) *float64 { return &s.Reverb }, pctText},
		{"CHO", 0, 2, 1, false, func(s *synth.GMStrip) *float64 { return &s.Chorus }, pctText},
		{"DLY", 0, 1, 0, false, func(s *synth.GMStrip) *float64 { return &s.Delay }, pctText},
		{"PAN", -1, 1, 0, true, func(s *synth.GMStrip) *float64 { return &s.Pan }, panText},
	}
	top := r.Min.Y + 44
	for i, d := range defs {
		row, col2 := i/2, i%2
		cx := r.Min.X + r.Size().W*(0.28+0.44*float32(col2))
		cy := top + float32(row)*cell + ks/2
		g := d.get
		m.ctls = append(m.ctls, mctl{kind: kKnob, r: xyxy(cx-ks/2, cy-ks/2, cx+ks/2, cy+ks/2), label: d.label,
			lo: d.lo, hi: d.hi, def: d.def, bipolar: d.bipolar, format: d.format, col: col, ch: ch,
			get: func(mm *synth.GMMix) float64 { return *g(strip(mm)) },
			set: func(mm *synth.GMMix, v float64) { *g(strip(mm)) = v }})
	}
	y := top + 4*cell
	bw := (r.Size().W - 16) / 2
	m.ctls = append(m.ctls,
		mctl{kind: kButton, r: xyxy(r.Min.X+6, y, r.Min.X+6+bw, y+22), label: "M", ch: ch, col: col,
			act: func(u *gunim.UI, n gunim.Node) { u.Send(n, ChannelClicked{Channel: ch}) }},
		mctl{kind: kButton, r: xyxy(r.Max.X-6-bw, y, r.Max.X-6, y+22), label: "S", ch: ch, col: col,
			act: func(u *gunim.UI, n gunim.Node) { u.Send(n, ChannelClicked{Channel: ch, Solo: true}) }})
	fy := y + 32
	fx := r.Min.X + r.Size().W*0.62
	fr := xyxy(fx-12, fy, fx+12, r.Max.Y-30)
	m.strips, m.stripCh, m.faders = append(m.strips, r), append(m.stripCh, ch), append(m.faders, fr)
	m.ctls = append(m.ctls, mctl{kind: kFader, r: fr, label: "", lo: -60, hi: 12, def: 0,
		format: dbText, col: col, ch: ch,
		get: func(mm *synth.GMMix) float64 { return strip(mm).Gain },
		set: func(mm *synth.GMMix, v float64) { strip(mm).Gain = v }})
}

// layoutMaster puts the master's controls in r.
func (m *mixer) layoutMaster(r geom.Rect) {
	pal := m.l.pal
	acc := pal.accent
	eqH := min(float32(190), r.Size().H*0.36)
	m.eqRect = xyxy(r.Min.X+12, r.Min.Y+34, r.Max.X-12, r.Min.Y+34+eqH)
	ranges := [5][2]float64{{20, 500}, {40, 2000}, {150, 8000}, {800, 16000}, {2000, 20000}}
	for b := range 5 {
		band := b
		m.ctls = append(m.ctls, mctl{kind: kEQ, band: b, ch: -1, col: pal.ch[(b*3+1)%16], lo: ranges[b][0], hi: ranges[b][1],
			get: func(mm *synth.GMMix) float64 { return mm.EQ[band].Gain },
			set: func(mm *synth.GMMix, v float64) { mm.EQ[band].Gain = v }})
	}
	// Under the equaliser: the effects, then the dynamics, and the
	// master fader at the right.
	area := xyxy(r.Min.X+12, m.eqRect.Max.Y+16, r.Max.X-92, r.Max.Y-6)
	rowH := (area.Size().H - 12) / 2
	knob := func(cx, cy float32, label string, lo, hi, def float64, format func(float64) string,
		get func(mm *synth.GMMix) *float64) {
		ks := min(float32(34), rowH*0.42)
		m.ctls = append(m.ctls, mctl{kind: kKnob, r: xyxy(cx-ks/2, cy-ks/2, cx+ks/2, cy+ks/2), label: label,
			lo: lo, hi: hi, def: def, format: format, col: acc, ch: -1,
			get: func(mm *synth.GMMix) float64 { return *get(mm) },
			set: func(mm *synth.GMMix, v float64) { *get(mm) = v }})
	}
	toggle := func(x, y float32, get func(mm *synth.GMMix) *bool) {
		m.ctls = append(m.ctls, mctl{kind: kToggle, r: xyxy(x, y, x+40, y+20), col: acc, ch: -1,
			get: func(mm *synth.GMMix) float64 {
				if *get(mm) {
					return 1
				}
				return 0
			},
			set: func(mm *synth.GMMix, v float64) { *get(mm) = v > 0.5 }})
	}
	secs := func(v float64) string { return fmt.Sprintf("%.0f ms", v*1000) }
	sizeText := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	decayText := func(v float64) string { return fmt.Sprintf("%.1f s", v) }
	ratioText := func(v float64) string { return fmt.Sprintf("%.1f:1", v) }
	// The effects row: reverb (4), delay (4), chorus (1).
	y0 := area.Min.Y
	cy := y0 + rowH*0.55
	unit := area.Size().W / 9.6
	x := area.Min.X
	for i, k := range []struct {
		label       string
		lo, hi, def float64
		f           func(float64) string
		get         func(mm *synth.GMMix) *float64
	}{
		{"SIZE", 0.3, 1.5, 0.9, sizeText, func(mm *synth.GMMix) *float64 { return &mm.Reverb.Size }},
		{"DECAY", 0.2, 8, 2.2, decayText, func(mm *synth.GMMix) *float64 { return &mm.Reverb.Decay }},
		{"TONE", 0, 1, 0.5, pctText, func(mm *synth.GMMix) *float64 { return &mm.Reverb.Tone }},
		{"RETURN", 0, 2, 1, pctText, func(mm *synth.GMMix) *float64 { return &mm.Reverb.Return }},
		{"TIME", 0.05, 1.5, 0.375, secs, func(mm *synth.GMMix) *float64 { return &mm.Delay.Time }},
		{"FEEDBACK", 0, 0.9, 0.35, pctText, func(mm *synth.GMMix) *float64 { return &mm.Delay.Feedback }},
		{"TONE", 0, 1, 0.5, pctText, func(mm *synth.GMMix) *float64 { return &mm.Delay.Tone }},
		{"RETURN", 0, 2, 1, pctText, func(mm *synth.GMMix) *float64 { return &mm.Delay.Return }},
		{"RETURN", 0, 2, 1, pctText, func(mm *synth.GMMix) *float64 { return &mm.Chorus.Return }},
	} {
		gap := float32(0)
		if i >= 4 {
			gap += 0.3 * unit
		}
		if i >= 8 {
			gap += 0.3 * unit
		}
		knob(x+gap+(float32(i)+0.5)*unit, cy, k.label, k.lo, k.hi, k.def, k.f, k.get)
	}
	// The dynamics row: the compressor (3) and the limiter (1).
	y1 := y0 + rowH + 12
	cy = y1 + rowH*0.55
	unit = area.Size().W / 7
	toggle(area.Min.X+area.Size().W*0.5-50, y1+6, func(mm *synth.GMMix) *bool { return &mm.Comp.On })
	toggle(area.Max.X-46, y1+6, func(mm *synth.GMMix) *bool { return &mm.Limiter.On })
	knob(area.Min.X+0.5*unit, cy, "THRESHOLD", -40, 0, -12, dbText, func(mm *synth.GMMix) *float64 { return &mm.Comp.Threshold })
	knob(area.Min.X+1.5*unit, cy, "RATIO", 1, 10, 2, ratioText, func(mm *synth.GMMix) *float64 { return &mm.Comp.Ratio })
	knob(area.Min.X+2.5*unit, cy, "MAKEUP", 0, 18, 0, dbText, func(mm *synth.GMMix) *float64 { return &mm.Comp.Makeup })
	knob(area.Min.X+4.6*unit, cy, "CEILING", -12, 0, -1, dbText, func(mm *synth.GMMix) *float64 { return &mm.Limiter.Ceiling })
	// The master fader, and Reset under it.
	fx := r.Max.X - 46
	m.ctls = append(m.ctls, mctl{kind: kFader, r: xyxy(fx-12, area.Min.Y+4, fx+12, r.Max.Y-40), lo: -24, hi: 12, def: 0,
		format: dbText, col: acc, ch: -1,
		get: func(mm *synth.GMMix) float64 { return mm.Gain },
		set: func(mm *synth.GMMix, v float64) { mm.Gain = v }})
	m.ctls = append(m.ctls, mctl{kind: kButton, r: xyxy(r.Max.X-84, r.Max.Y-30, r.Max.X-10, r.Max.Y-6), label: "Reset", ch: -1, col: acc,
		act: func(u *gunim.UI, n gunim.Node) {
			m.local = synth.DefaultGMMix()
			m.edited(u)
		}})
}

// eqAt returns where band b's handle is in the equaliser's graph.
func (m *mixer) eqAt(b synth.GMBand) geom.Point {
	r := m.eqRect
	x := r.Min.X + r.Size().W*float32(math.Log(b.Freq/20)/math.Log(1000))
	y := r.Min.Y + r.Size().H*(0.5-float32(b.Gain/36))
	return geom.Pt(x, y)
}

// ctlAt returns the control at pos, -1 for none.
func (m *mixer) ctlAt(pos geom.Point) int {
	for i, c := range m.ctls {
		// A strip scrolled out of its room takes nothing.
		if c.ch >= 0 && (pos.X > m.stripsW || pos.X < 0) {
			continue
		}
		if c.kind == kEQ {
			at := m.eqAt(m.local.EQ[c.band])
			if d := pos.Sub(at); d.X*d.X+d.Y*d.Y < 12*12 {
				return i
			}
			continue
		}
		hit := c.r.Inset(geom.Uniform(-4))
		if c.kind == kKnob {
			hit.Max.Y += 14
		}
		if hit.Contains(pos) {
			return i
		}
	}
	return -1
}

// edited sends the mix as the desk has it.
func (m *mixer) edited(u *gunim.UI) {
	m.lastEdit = time.Now()
	u.Send(m, MixSet{Mix: m.local})
	u.Invalidate()
}

// faderPos returns where v sits along a fader from lo to hi, from 0 at
// the bottom to 1 at the top: the decibels near 0 spread out, as a
// desk's are.
func faderPos(v, lo, hi float64) float32 {
	x := (v - lo) / (hi - lo)
	return float32(math.Pow(min(max(x, 0), 1), 1.6))
}

func faderValue(pos float32, lo, hi float64) float64 {
	return lo + math.Pow(float64(min(max(pos, 0), 1)), 1/1.6)*(hi-lo)
}

func (m *mixer) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if m.drag >= 0 && m.drag < len(m.ctls) {
			c := m.ctls[m.drag]
			fine := float64(1)
			if e.Mods&input.ModShift != 0 {
				fine = 0.2
			}
			dy := float64(m.dragY - e.Pos.Y)
			switch c.kind {
			case kKnob:
				c.set(&m.local, min(max(m.from+dy*(c.hi-c.lo)/180*fine, c.lo), c.hi))
			case kFader:
				p := faderPos(m.from, c.lo, c.hi) + float32(dy*fine)/c.r.Size().H
				c.set(&m.local, faderValue(p, c.lo, c.hi))
			case kEQ:
				r := m.eqRect
				f := 20 * math.Pow(1000, float64((e.Pos.X-r.Min.X)/r.Size().W))
				g := float64(0.5-(e.Pos.Y-r.Min.Y)/r.Size().H) * 36
				band := &m.local.EQ[c.band]
				band.Freq = min(max(f, c.lo), c.hi)
				band.Gain = math.Round(min(max(g, -15), 15)*10) / 10
			}
			m.edited(u)
			return true
		}
		if h := m.ctlAt(e.Pos); h != m.hover {
			m.hover = h
			u.Invalidate()
		}
		return false
	case input.PointerLeave:
		m.hover = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := m.ctlAt(e.Pos)
		if i < 0 {
			return false
		}
		c := m.ctls[i]
		double := i == m.lastClicked && time.Since(m.lastClick) < 350*time.Millisecond
		m.lastClick, m.lastClicked = time.Now(), i
		switch c.kind {
		case kKnob, kFader:
			if double {
				c.set(&m.local, c.def)
				m.edited(u)
				return true
			}
			m.drag, m.dragX, m.dragY, m.from = i, e.Pos.X, e.Pos.Y, c.get(&m.local)
			// A press on a fader's track, off its cap, jumps the cap there.
			if c.kind == kFader {
				capY := c.r.Max.Y - c.r.Size().H*faderPos(m.from, c.lo, c.hi)
				if math.Abs(float64(e.Pos.Y-capY)) > 14 {
					m.from = faderValue((c.r.Max.Y-e.Pos.Y)/c.r.Size().H, c.lo, c.hi)
					c.set(&m.local, m.from)
					m.edited(u)
				}
			}
		case kEQ:
			if double {
				m.local.EQ[c.band].Gain = 0
				m.edited(u)
				return true
			}
			m.drag = i
		case kToggle:
			c.set(&m.local, 1-c.get(&m.local))
			m.edited(u)
		case kButton:
			c.act(u, m)
		}
		return true
	case input.PointerUp:
		if m.drag >= 0 {
			m.drag = -1
			m.lastEdit = time.Now()
			return true
		}
		return false
	case input.Scroll:
		i := m.ctlAt(e.Pos)
		if i >= 0 && e.Mods&input.ModShift == 0 {
			c := m.ctls[i]
			step := float64(e.Delta.Y) / 400
			switch c.kind {
			case kKnob:
				c.set(&m.local, min(max(c.get(&m.local)+step*(c.hi-c.lo), c.lo), c.hi))
			case kFader:
				c.set(&m.local, faderValue(faderPos(c.get(&m.local), c.lo, c.hi)+float32(step), c.lo, c.hi))
			case kEQ:
				band := &m.local.EQ[c.band]
				band.Q = min(max(band.Q*math.Pow(2, step*2), 0.3), 6)
			default:
				return false
			}
			m.edited(u)
			return true
		}
		if e.Pos.X < m.stripsW {
			d := e.Delta.X
			if d == 0 {
				d = e.Delta.Y
			}
			m.scroll -= d
			u.Invalidate()
			return true
		}
		return false
	default:
		return false
	}
	return true
}

func (m *mixer) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := m.l
	// The desk draws crisp in every style: its knobs and meters are too
	// fine for a console's fat pixels.
	pal := l.pal
	pal.pixel = 0
	dt := float32(f.Delta.Seconds())
	m.fall(dt)
	if box.W < 200 || box.H < 200 {
		return
	}
	mw := min(max(box.W*0.42, masterMinW), 600)
	// The strips' room, clipped and faded at its ends as it scrolls.
	rad := 18 * pal.round
	strips := xyxy(0, 0, m.stripsW, box.H)
	p.ShadowRRect(strips, rad, paint.Solid(alpha(pal.panel, 0.92)), paint.Shadow{Blur: 20, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.35)})
	p.RRectStroke(strips, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	fade := geom.Insets{}
	if m.scroll > 1 {
		fade.Left = 24
	}
	if m.scroll < m.stripsTotal-m.stripsW-1 {
		fade.Right = 24
	}
	end := p.Layer(paint.LayerOpts{Bounds: strips, Opacity: 1, Clip: true, Radius: rad, Fade: fade})
	for i, r := range m.strips {
		m.paintStripFrame(p, m.stripCh[i], r, m.faders[i], pal)
	}
	for i := range m.ctls {
		if m.ctls[i].ch >= 0 {
			m.paintCtl(p, i, pal)
		}
	}
	end()
	master := xyxy(box.W-mw, 0, box.W, box.H)
	p.ShadowRRect(master, rad, paint.Solid(alpha(pal.panel, 0.95)), paint.Shadow{Blur: 20, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.35)})
	p.RRectStroke(master, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	shaped("MASTER", 13, true).Paint(p, geom.Pt(master.Min.X+14, 10), pal.ink)
	shaped("EQ", 11, false).Paint(p, geom.Pt(master.Min.X+76, 12), pal.dim)
	m.paintEQ(p, pal)
	m.paintMasterCards(p, master, pal)
	for i := range m.ctls {
		if m.ctls[i].ch < 0 {
			m.paintCtl(p, i, pal)
		}
	}
}

// fall lets the meters fall, and takes the latest readings.
func (m *mixer) fall(dt float32) {
	mt := m.l.s.Meters
	k := float32(math.Pow(10, float64(-dt*24/20))) // 24 dB a second
	hk := float32(math.Pow(10, float64(-dt*6/20)))
	for i := range m.ch {
		m.ch[i] = max(m.ch[i]*k, mt.Channels[i])
		m.hold[i] = max(m.hold[i]*hk, m.ch[i])
	}
	m.left, m.right = max(m.left*k, mt.Left), max(m.right*k, mt.Right)
	m.lHold, m.rHold = max(m.lHold*hk, m.left), max(m.rHold*hk, m.right)
	m.comp += (mt.Comp - m.comp) * min(1, dt*10)
	m.limit += (mt.Limit - m.limit) * min(1, dt*10)
	if len(m.spec) != len(m.l.s.Spectrum) {
		m.spec = make([]float32, len(m.l.s.Spectrum))
	}
	for i, v := range m.l.s.Spectrum {
		if v > m.spec[i] {
			m.spec[i] += (v - m.spec[i]) * min(1, dt*25)
		} else {
			m.spec[i] += (v - m.spec[i]) * min(1, dt*6)
		}
	}
}

// meterPos returns where an amplitude sits on a meter, from 0 at -60
// dB to 1 at +6.
func meterPos(a float32) float32 {
	if a <= 0 {
		return 0
	}
	db := 20 * math.Log10(float64(a))
	return float32(min(max((db+60)/66, 0), 1))
}

// paintMeter draws a meter of amplitude a in r, holding its peak at
// hold: green, then yellow from -12 dB, then red from -3.
func (m *mixer) paintMeter(p *paint.Painter, r geom.Rect, a, hold float32, pal palette) {
	pn := pen{pal: pal}
	pn.rect(p, r, 2, mix(pal.panel, pal.ink, 0.08))
	lit := meterPos(a)
	h := r.Size().H
	if lit > 0 {
		top := r.Max.Y - h*lit
		y12 := r.Max.Y - h*meterPos(float32(math.Pow(10, -12.0/20)))
		y3 := r.Max.Y - h*meterPos(float32(math.Pow(10, -3.0/20)))
		green, yellow, red := rgb(0x3ddc84), rgb(0xffd23f), rgb(0xff4d4d)
		if pal.mono > 0.5 {
			// Four greens have no yellow or red: the meter darkens as it
			// climbs.
			green, yellow, red = pal.dim, pal.ink, pal.ink
		}
		pn.rect(p, xyxy(r.Min.X, max(top, y12), r.Max.X, r.Max.Y), 1, green)
		if top < y12 {
			pn.rect(p, xyxy(r.Min.X, max(top, y3), r.Max.X, y12), 0, yellow)
		}
		if top < y3 {
			pn.rect(p, xyxy(r.Min.X, top, r.Max.X, y3), 0, red)
		}
	}
	if hp := meterPos(hold); hp > 0.01 {
		y := r.Max.Y - h*hp
		c := pal.ink
		if hold > 1 {
			c = rgb(0xff4d4d)
		}
		pn.rect(p, xyxy(r.Min.X, y-1, r.Max.X, y+1), 0, c)
	}
}

// paintStripFrame draws channel ch's strip, in r, behind its controls:
// its card, its name, its meter and its scale. fader is where its
// fader is.
func (m *mixer) paintStripFrame(p *paint.Painter, ch int, r, fader geom.Rect, pal palette) {
	l := m.l
	r = r.Inset(geom.Insets{Top: 6, Bottom: 6})
	col := pal.ch[ch]
	s := l.s
	muted := s.Muted[ch] || s.Solo >= 0 && s.Solo != ch
	lit := min(1, l.ch[ch].level)
	p.RRect(r, 12*pal.round, paint.Fill{Gradient: &paint.Gradient{From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y),
		Start: lighten(pal.panel, 0.05), End: mix(pal.panel, col, 0.08+0.12*lit)}})
	if s.Solo == ch {
		p.RRectStroke(r, 12*pal.round, paint.Fill{}, paint.Stroke{Width: 2, Color: pal.accent})
	}
	badge := shaped(fmt.Sprint(ch+1), 11, true)
	bw := max(badge.Advance+10, 20)
	p.RRect(xyxy(r.Min.X+8, r.Min.Y+8, r.Min.X+8+bw, r.Min.Y+26), 9*pal.round, paint.Solid(col))
	badge.Paint(p, geom.Pt(r.Min.X+8+(bw-badge.Advance)/2, r.Min.Y+10), darken(col, 0.75))
	name := "Channel"
	if sc := l.score; sc != nil {
		if sc.Channels[ch].Drums {
			name = "Drums"
		} else {
			name = synth.GMNames[l.ch[ch].program&127]
		}
	}
	ink := pal.ink
	if muted {
		ink = alpha(pal.ink, 0.4)
	}
	fit(name, 10, true, r.Size().W-bw-18).Paint(p, geom.Pt(r.Min.X+bw+13, r.Min.Y+11), ink)
	// The meter, left of the fader, and its scale.
	mr := xyxy(r.Min.X+10, fader.Min.Y, r.Min.X+18, fader.Max.Y)
	m.paintMeter(p, mr, m.ch[ch], m.hold[ch], pal)
	for _, db := range []float64{0, -12, -24, -48} {
		y := fader.Max.Y - fader.Size().H*faderPos(db, -60, 12)
		p.RRect(xyxy(fader.Min.X-2, y, fader.Min.X+2, y+1), 0, paint.Solid(alpha(pal.dim, 0.6)))
	}
}

// paintCtl draws control i.
func (m *mixer) paintCtl(p *paint.Painter, i int, pal palette) {
	c := &m.ctls[i]
	hot := i == m.hover || i == m.drag
	pn := pen{pal: pal}
	switch c.kind {
	case kKnob:
		v := c.get(&m.local)
		cx, cy := c.r.Min.X+c.r.Size().W/2, c.r.Min.Y+c.r.Size().H/2
		rad := c.r.Size().W / 2
		body := mix(pal.panel, pal.ink, 0.12)
		if hot {
			body = mix(pal.panel, pal.ink, 0.2)
		}
		// The arc of the value, round the knob, from 7 o'clock to 5.
		const a0, a1 = 0.75 * math.Pi, 2.25 * math.Pi
		t := (v - c.lo) / (c.hi - c.lo)
		from, to := a0, a0+(a1-a0)*t
		if c.bipolar {
			from = (a0 + a1) / 2
		}
		if from > to {
			from, to = to, from
		}
		for k := range 25 {
			a := a0 + (a1-a0)*float64(k)/24
			on := a >= from-0.01 && a <= to+0.01
			col := alpha(pal.edge, 0.5)
			if on {
				col = c.col
			}
			x, y := cx+cosf(a)*(rad+3), cy+sinf(a)*(rad+3)
			pn.rect(p, xyxy(x-1.3, y-1.3, x+1.3, y+1.3), 1.3, col)
		}
		pn.circle(p, cx, cy, rad-2, body)
		a := a0 + (a1-a0)*t
		pn.line(p, geom.Pt(cx+cosf(a)*rad*0.2, cy+sinf(a)*rad*0.2), geom.Pt(cx+cosf(a)*(rad-4), cy+sinf(a)*(rad-4)), 2, pal.ink)
		label := c.label
		col := alpha(pal.dim, 0.9)
		if hot {
			label, col = c.format(v), pal.ink
		}
		room := c.r.Size().W + 22
		if c.ch >= 0 {
			room = c.r.Size().W + 8
		}
		run := fit(label, 8.5, hot, room)
		run.Paint(p, geom.Pt(cx-run.Advance/2, c.r.Max.Y+5), col)
	case kFader:
		v := c.get(&m.local)
		mid := c.r.Min.X + c.r.Size().W/2
		pn.rect(p, xyxy(mid-2, c.r.Min.Y, mid+2, c.r.Max.Y), 2, alpha(pal.bgBottom, 0.8))
		zero := c.r.Max.Y - c.r.Size().H*faderPos(0, c.lo, c.hi)
		pn.rect(p, xyxy(mid-8, zero, mid+8, zero+1), 0, alpha(pal.dim, 0.7))
		y := c.r.Max.Y - c.r.Size().H*faderPos(v, c.lo, c.hi)
		capC := mix(pal.panel, pal.ink, 0.35)
		if hot {
			capC = mix(pal.panel, pal.ink, 0.5)
		}
		p.ShadowRRect(xyxy(c.r.Min.X, y-9, c.r.Max.X, y+9), 4*pal.round, paint.Solid(pal.tone(capC)),
			paint.Shadow{Blur: 6, Offset: geom.Pt(0, 2), Color: alpha(color.NRGBA{A: 255}, 0.5)})
		pn.rect(p, xyxy(c.r.Min.X+3, y-1, c.r.Max.X-3, y+1), 0, c.col)
		run := shaped(c.format(v), 10, true)
		run.Paint(p, geom.Pt(mid-run.Advance/2, c.r.Max.Y+8), pal.ink)
	case kToggle:
		on := c.get(&m.local) > 0.5
		fill := alpha(pal.bgBottom, 0.7)
		ink := pal.dim
		if on {
			fill, ink = c.col, darken(c.col, 0.75)
		}
		pn.rect(p, c.r, 10, fill)
		if pal.mono > 0.5 && !on {
			p.RRectStroke(c.r, 0, paint.Fill{}, paint.Stroke{Width: 1, Color: pal.dim})
		}
		label := "OFF"
		if on {
			label = "ON"
		}
		run := shaped(label, 10, true)
		run.Paint(p, geom.Pt(c.r.Min.X+(c.r.Size().W-run.Advance)/2, c.r.Min.Y+(c.r.Size().H-run.Height())/2), ink)
	case kButton:
		s := m.l.s
		fill := mix(pal.panel, pal.ink, 0.12)
		ink := pal.ink
		switch c.label {
		case "M":
			if s.Muted[c.ch] {
				fill, ink = rgb(0xff5c5c), rgb(0x2a0a0a)
			}
		case "S":
			if s.Solo == c.ch {
				fill, ink = pal.accent, darken(pal.accent, 0.8)
			}
		}
		if hot {
			fill = mix(fill, pal.ink, 0.1)
		}
		pn.rect(p, c.r, 6, fill)
		if pal.mono > 0.5 {
			p.RRectStroke(c.r, 0, paint.Fill{}, paint.Stroke{Width: 1, Color: pal.dim})
		}
		if c.label == "Reset" {
			sz := float32(12)
			ix := c.r.Min.X + 10
			iy := c.r.Min.Y + (c.r.Size().H-sz)/2
			p.Mask(icon.Stroke{Icon: icon.RotateCcw, Width: 2, Progress: 1}, xyxy(ix, iy, ix+sz, iy+sz), pal.tone(ink))
			run := shaped("Reset", 11, true)
			run.Paint(p, geom.Pt(ix+sz+6, c.r.Min.Y+(c.r.Size().H-run.Height())/2), ink)
			return
		}
		run := shaped(c.label, 11, true)
		run.Paint(p, geom.Pt(c.r.Min.X+(c.r.Size().W-run.Advance)/2, c.r.Min.Y+(c.r.Size().H-run.Height())/2), ink)
	case kEQ:
		b := m.local.EQ[c.band]
		at := m.eqAt(b)
		rad := float32(7)
		if hot {
			rad = 9
		}
		pn.glow(p, at.X, at.Y, 22, alpha(c.col, 0.5))
		pn.circle(p, at.X, at.Y, rad, c.col)
		pn.circle(p, at.X, at.Y, rad-3, darken(c.col, 0.4))
		if hot {
			tip := shaped(fmt.Sprintf("%s Hz  %s dB  Q %.1f", hzText(b.Freq), dbText(b.Gain), b.Q), 11, true)
			tx := min(max(at.X-tip.Advance/2, m.eqRect.Min.X), m.eqRect.Max.X-tip.Advance)
			ty := at.Y - 28
			if ty < m.eqRect.Min.Y {
				ty = at.Y + 14
			}
			pn.rect(p, xyxy(tx-6, ty-3, tx+tip.Advance+6, ty+tip.Height()+3), 5, alpha(pal.bgBottom, 0.9))
			tip.Paint(p, geom.Pt(tx, ty), pal.ink)
		}
	}
}

// paintEQ draws the master equaliser: its grid, the sound heard as a
// spectrum, and the curve the bands make together.
func (m *mixer) paintEQ(p *paint.Painter, pal palette) {
	r := m.eqRect
	pn := pen{pal: pal}
	p.RRect(r, 10*pal.round, paint.Solid(alpha(pal.bgBottom, 0.6)))
	end := p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 10 * pal.round})
	defer end()
	xOf := func(hz float64) float32 { return r.Min.X + r.Size().W*float32(math.Log(hz/20)/math.Log(1000)) }
	yOf := func(db float64) float32 { return r.Min.Y + r.Size().H*(0.5-float32(db/36)) }
	for _, hz := range []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000} {
		x := xOf(hz)
		p.RRect(xyxy(x, r.Min.Y, x+1, r.Max.Y), 0, paint.Solid(alpha(pal.edge, 0.3)))
		lab := shaped(hzText(hz), 9, false)
		lab.Paint(p, geom.Pt(x+3, r.Max.Y-lab.Height()-3), alpha(pal.dim, 0.7))
	}
	for _, db := range []float64{-12, -6, 0, 6, 12} {
		y := yOf(db)
		a := float32(0.25)
		if db == 0 {
			a = 0.6
		}
		p.RRect(xyxy(r.Min.X, y, r.Max.X, y+1), 0, paint.Solid(alpha(pal.edge, a)))
	}
	// The spectrum, as heard: a soft hill from the bottom, -90 dB to
	// -6 over the height.
	if n := len(m.spec); n > 1 {
		hOf := func(v float32) float32 { return float32(min(max((float64(v)+90)/84, 0), 1)) * r.Size().H }
		for i := 0; i+1 < n; i++ {
			x0, x1 := xOf(float64(specFreqs[i])), xOf(float64(specFreqs[i+1]))
			h0, h1 := hOf(m.spec[i]), hOf(m.spec[i+1])
			c := mix(pal.accent2, pal.accent, float32(i)/float32(n))
			p.RRect(xyxy(x0, r.Max.Y-(h0+h1)/2, x1+0.6, r.Max.Y), 0, paint.Fill{Gradient: &paint.Gradient{
				From: geom.Pt(0, r.Max.Y-r.Size().H), To: geom.Pt(0, r.Max.Y), Start: alpha(pal.tone(c), 0.4), End: alpha(pal.tone(c), 0.05)}})
			pn.line(p, geom.Pt(x0, r.Max.Y-h0), geom.Pt(x1, r.Max.Y-h1), 1.2, alpha(c, 0.7))
		}
	}
	// The curve.
	const steps = 120
	var prev geom.Point
	for k := 0; k <= steps; k++ {
		hz := 20 * math.Pow(1000, float64(k)/steps)
		db := 0.0
		for b, band := range m.local.EQ {
			if band.Gain != 0 {
				db += synth.GMBandResponse(b, band, hz)
			}
		}
		pt := geom.Pt(xOf(hz), yOf(db))
		if k > 0 {
			zero := yOf(0)
			top, bot := min(pt.Y, zero), max(pt.Y, zero)
			p.RRect(xyxy(prev.X, top, pt.X+0.5, bot), 0, paint.Solid(alpha(pal.accent, 0.12)))
			pn.line(p, prev, pt, 2.5, pal.accent)
		}
		prev = pt
	}
}

// paintMasterCards draws the cards behind the master's knobs, their
// names, and the meters of the compressor, the limiter and the master.
func (m *mixer) paintMasterCards(p *paint.Painter, master geom.Rect, pal palette) {
	area := xyxy(master.Min.X+12, m.eqRect.Max.Y+16, master.Max.X-92, master.Max.Y-6)
	rowH := (area.Size().H - 12) / 2
	card := func(r geom.Rect, title string) {
		p.RRect(r, 10*pal.round, paint.Solid(mix(pal.panel, pal.ink, 0.04)))
		if pal.mono > 0.5 {
			p.RRectStroke(r, 0, paint.Fill{}, paint.Stroke{Width: 1, Color: pal.dim})
		}
		shaped(title, 10, true).Paint(p, geom.Pt(r.Min.X+10, r.Min.Y+7), pal.dim)
	}
	unit := area.Size().W / 9.6
	y0 := area.Min.Y
	card(xyxy(area.Min.X, y0, area.Min.X+4*unit, y0+rowH), "REVERB")
	card(xyxy(area.Min.X+4.3*unit, y0, area.Min.X+8.3*unit, y0+rowH), "DELAY")
	card(xyxy(area.Min.X+8.6*unit, y0, area.Max.X, y0+rowH), "CHORUS")
	y1 := y0 + rowH + 12
	unit = area.Size().W / 7
	card(xyxy(area.Min.X, y1, area.Min.X+3.6*unit, y1+rowH), "COMPRESSOR")
	card(xyxy(area.Min.X+3.8*unit, y1, area.Max.X, y1+rowH), "LIMITER")
	// How far each turns the sound down: a bar falling from the top.
	gr := func(x float32, db float32, on bool) {
		r := xyxy(x, y1+30, x+8, y1+rowH-10)
		pen{pal: pal}.rect(p, r, 2, alpha(pal.bgBottom, 0.7))
		if on && db > 0.05 {
			h := r.Size().H * min(db/20, 1)
			pen{pal: pal}.rect(p, xyxy(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h), 1, rgb(0xff9f3f))
		}
		lab := shaped(fmt.Sprintf("−%.1f", max(db, 0)), 9, true)
		if !on {
			lab = shaped("GR", 9, true)
		}
		lab.Paint(p, geom.Pt(x+4-lab.Advance/2, y1+rowH-9), pal.dim)
	}
	gr(area.Min.X+3.6*unit-16, m.comp, m.local.Comp.On)
	gr(area.Max.X-16, m.limit, m.local.Limiter.On)
	// The master's meters, either side of its fader.
	mx := master.Max.X - 46
	top, bot := area.Min.Y+4, master.Max.Y-40
	m.paintMeter(p, xyxy(mx-26, top, mx-20, bot), m.left, m.lHold, pal)
	m.paintMeter(p, xyxy(mx+20, top, mx+26, bot), m.right, m.rHold, pal)
	shaped("L", 9, true).Paint(p, geom.Pt(mx-26, top-13), pal.dim)
	shaped("R", 9, true).Paint(p, geom.Pt(mx+20, top-13), pal.dim)
}
