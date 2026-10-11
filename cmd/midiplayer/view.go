package main

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// look is what every part of the window draws from, worked out once a
// frame by the root: the state, the palette as it blends from style to
// style, the time heard, and what each channel plays then.
type look struct {
	s     Player
	score *Score
	pal   palette
	// t is the time heard, in seconds, smoothed between the clock's
	// readings, and now the frame's time.
	t   float64
	now time.Time
	// beat is the beat heard, from 0, phase how far through it, and pulse
	// a flash on each beat, from 1 falling to 0, stronger on a bar's
	// first.
	beat  int
	phase float64
	pulse float32
	// ch is what each channel plays now.
	ch [16]chanLive
	// fresh counts up from 0 to 1 over the second after a song loads.
	fresh float32
	// keyX says where on the window each key of the piano is, from its
	// left edge to its right, for the sparks; keyY is the keys' top.
	keyX [128][2]float32
	keyY float32
	// station says where each channel's station is on the window, empty
	// for none.
	station [16]geom.Rect
	// playing says the song moves, so everything animates.
	playing bool
}

// chanLive is what a channel plays at the time heard.
type chanLive struct {
	notes []liveNote
	// level is how loud it is, easing, and hit flashes as a note
	// starts, falling from 1.
	level, hit float32
	program    int
	// struck counts the notes it has started, for an animation that
	// changes with each.
	struck int
}

// liveNote is a note sounding, or just let go: age is how long since it
// started, and left how long until it ends, negative once it has.
type liveNote struct {
	key       uint8
	vel       float32
	age, left float64
}

// update works out the look at frame time now, dt after the last.
func (l *look) update(now time.Time, dt float32) {
	l.now = now
	target := l.s.Clock.heard(now)
	switch {
	case l.s.Clock.Rate == 0 || math.Abs(target-l.t) > 0.25:
		l.t = target
	default:
		// The clock's readings come a few times a second; between them
		// the time runs on, and leans toward each reading as it comes.
		l.t += float64(dt)*l.s.Clock.Rate + (target-l.t-float64(dt)*l.s.Clock.Rate)*float64(min(1, dt*8))
	}
	l.playing = l.s.Playing
	l.fresh = min(1, l.fresh+dt)
	sc := l.score
	for i := range l.ch {
		l.ch[i].notes = l.ch[i].notes[:0]
	}
	if sc == nil {
		for i := range l.ch {
			l.ch[i].level *= float32(math.Exp(float64(-dt * 4)))
			l.ch[i].hit = max(0, l.ch[i].hit-dt*4)
		}
		l.pulse = max(0, l.pulse-dt*3)
		return
	}
	beat, phase := sc.beatAt(l.t)
	if beat != l.beat && l.playing {
		l.pulse = 0.6
		if sc.BarBeats > 0 && beat%sc.BarBeats == 0 {
			l.pulse = 1
		}
	}
	l.beat, l.phase = beat, phase
	l.pulse = max(0, l.pulse-dt*2.5)
	var energy [16]float32
	sc.span(l.t-0.6, l.t, func(n *Note) {
		c := &l.ch[n.Ch]
		ln := liveNote{key: n.Key, vel: float32(n.Vel) / 127, age: l.t - n.Start, left: n.End - l.t}
		if ln.age < 0 {
			return
		}
		c.notes = append(c.notes, ln)
		e := ln.vel * ln.vel
		if ln.left > 0 {
			e *= 0.45 + 0.55*float32(math.Exp(-ln.age*3))
		} else {
			e *= 0.45 * float32(math.Exp(ln.left*8))
		}
		energy[n.Ch] += e
		if ln.age < float64(dt)+0.001 && l.playing {
			c.hit = 1
			c.struck++
		}
	})
	for i := range l.ch {
		c := &l.ch[i]
		target := min(energy[i], 1.4)
		k := min(1, dt*18)
		if target < c.level {
			k = min(1, dt*5)
		}
		c.level += (target - c.level) * k
		c.hit = max(0, c.hit-dt*3.5)
		c.program = sc.programAt(i, l.t)
	}
}

// root is the window's view: it lays out the parts, draws the
// background and what lies over everything, and takes drops of files
// and the keys.
type root struct {
	l     *look
	deck  *deck
	band  *band
	roll  *roll
	trans *transport
	list  *playlist
	mixer *mixer
	tabs  *tabs
	// page is the page shown, the stage or the mixer, and pg slides
	// between them, from 0, the stage, to 1, the mixer.
	page     int
	pg       float32
	gen      int
	style    string
	from, to palette
	// blend runs from 0 to 1 as the palette changes style, and boot
	// shows the style's start-up screen, from 1 falling to 0.
	blend, boot float32
	bootStyle   string
	// listOpen slides the playlist in, from 0 to 1.
	listOpen float32
	// toast shows the latest message, from 1 falling to 0, and msgGen is
	// the message it shows.
	toast  float32
	msgGen int
	msg    string
	// drag is how far a drag of files over the window has lit it, and
	// dragOK says the files are MIDI files; dropped flashes as they land.
	drag, dropped float32
	dragOK        bool
	dragN         int
	size          geom.Size
	spin          float32
}

func buildView(s Player) *root {
	l := &look{s: s, score: s.Score, pal: palettes[s.Style]}
	r := &root{l: l, page: startPage, pg: float32(startPage), gen: s.ScoreGen, style: s.Style, from: palettes[s.Style], to: palettes[s.Style], blend: 1,
		msgGen: s.MessageGen}
	r.deck = &deck{l: l, over: -1, down: -1}
	r.band = &band{l: l, hover: -1, press: -1}
	r.roll = &roll{l: l}
	r.list = &playlist{l: l, over: -1, down: -1}
	r.trans = &transport{l: l, list: r.list, over: -1, down: -1, hoverAt: -1}
	r.mixer = &mixer{l: l, hover: -1, drag: -1, lastClicked: -1}
	r.tabs = &tabs{r: r, over: -1}
	if len(s.Playlist) > 1 {
		r.listOpen = 1
	}
	return r
}

// Children implements [gunim.Composite].
func (r *root) Children() []gunim.Node {
	return []gunim.Node{r.band, r.roll, r.list, r.deck, r.trans, r.mixer, r.tabs}
}

// update shows s.
func (r *root) update(s Player, u *gunim.UI) {
	l := r.l
	l.s = s
	if s.ScoreGen != r.gen {
		r.gen = s.ScoreGen
		l.score = s.Score
		l.fresh = 0
		l.t = s.Clock.Time
		r.band.reset()
	}
	if s.Style != r.style {
		r.from, r.to = l.pal, palettes[s.Style]
		r.style, r.blend = s.Style, 0
		r.boot, r.bootStyle = 1, s.Style
	}
	if s.MessageGen != r.msgGen && s.Message != "" {
		r.msgGen, r.msg, r.toast = s.MessageGen, s.Message, 1
	}
	if len(s.Playlist) > 1 && r.listOpen == 0 && !r.list.closed {
		r.list.open = true
	}
	u.Invalidate()
}

// Step moves what the root animates.
func (r *root) Step(dt time.Duration) bool {
	t := float32(dt.Seconds())
	r.blend = min(1, r.blend+t*1.6)
	r.l.pal = mixPalette(r.from, r.to, ease(r.blend))
	r.boot = max(0, r.boot-t*0.75)
	r.toast = max(0, r.toast-t*0.22)
	r.dropped = max(0, r.dropped-t*1.8)
	want := float32(0)
	if r.list.open {
		want = 1
	}
	r.listOpen += (want - r.listOpen) * min(1, t*10)
	if math.Abs(float64(want-r.listOpen)) < 0.002 {
		r.listOpen = want
	}
	r.spin += t
	pw := float32(r.page)
	r.pg += (pw - r.pg) * min(1, t*9)
	if math.Abs(float64(pw-r.pg)) < 0.002 {
		r.pg = pw
	}
	moving := r.pg != pw || r.page == 1 || r.l.playing || r.blend < 1 || r.boot > 0 || r.toast > 0 || r.dropped > 0 || r.drag > 0 ||
		r.listOpen != want || r.l.fresh < 1 || r.l.pulse > 0 || r.l.score == nil
	for _, c := range r.l.ch {
		moving = moving || c.level > 0.002 || c.hit > 0
	}
	return moving
}

// The layout's measures.
const (
	margin    = 16
	headerH   = 96
	transH    = 104
	listW     = 300
	minRollH  = 170
	rollShare = 0.40
)

func (r *root) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	r.size = size
	top := f.Safe.Top + 6
	w, h := size.W, size.H
	place := func(i int, rc geom.Rect) {
		k := kids.At(i)
		k.Layout(gunim.Tight(rc.Size()))
		k.Place(rc.Min)
	}
	// The deck of styles at the top right, under the title bar.
	deckW := min(float32(470), w*0.45)
	place(3, xyxy(w-margin-deckW, top, w-margin, top+headerH))
	// The transport along the bottom.
	place(4, xyxy(margin, h-margin-transH, w-margin, h-margin))
	midTop := top + headerH + 8
	midBot := h - margin - transH - 10
	rollH := max(minRollH, (midBot-midTop)*rollShare)
	panel := listW * ease(r.listOpen)
	bandRight := w - margin - panel
	if panel > 1 {
		bandRight -= 12 * ease(r.listOpen)
	}
	// The stage slides down and away as the mixer rises in its place.
	pg := ease(r.pg)
	off := (h + 40) * pg
	if r.pg >= 1 {
		off = 4 * h
	}
	r.band.bounds = xyxy(margin, midTop+off, bandRight, midBot-rollH-12+off)
	r.roll.bounds = xyxy(margin, midBot-rollH+off, w-margin, midBot+off)
	place(0, r.band.bounds)
	place(1, r.roll.bounds)
	moff := (h + 40) * (1 - pg)
	if r.pg <= 0 {
		moff = 4 * h
	}
	place(5, xyxy(margin, midTop+moff, bandRight, midBot+moff))
	// The tabs, left of the deck.
	place(6, xyxy(w-margin-deckW-206, top+30, w-margin-deckW-16, top+64))
	// The playlist slides in from the right, beside the band, and out
	// past the window's edge.
	x := w - margin - panel + (margin+30)*(1-ease(r.listOpen))
	place(2, xyxy(x, midTop, x+listW, midBot-rollH-12))
	return size
}

func (r *root) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	dt := float32(f.Delta.Seconds())
	r.l.update(f.Now, min(dt, 0.1))
	pal := r.l.pal
	full := geom.Rect{Max: box.Point()}
	// The background: shaded top to bottom, lit from the middle, the
	// light swelling on the beat.
	p.RRect(full, 0, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, 0), To: geom.Pt(0, box.H),
		Start: pal.bgTop, End: pal.bgBottom}})
	glow := alpha(pal.glow, 0.35+0.35*r.l.pulse)
	p.RRect(full, 0, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(box.W*0.5, box.H*0.42), To: geom.Pt(box.W*0.95, box.H),
		Radial: true, Start: glow, End: alpha(pal.glow, 0)}})
	r.paintStars(p, box)
	r.paintTitle(p, f, box)
	// The parts: the stage and the mixer each fade as they slide.
	pg := ease(r.pg)
	for i := range kids.Len() {
		var a float32 = 1
		switch i {
		case 0, 1:
			a = 1 - pg
		case 5:
			a = pg
		}
		if a <= 0.001 {
			continue
		}
		if a < 0.999 {
			end := p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: a})
			kids.At(i).Paint(p)
			end()
			continue
		}
		kids.At(i).Paint(p)
	}
	if r.pg < 0.5 {
		r.paintSparks(p)
	}
	r.paintBoot(p, box)
	r.paintToast(p, box)
	r.paintScan(p, box)
	r.paintDrop(p, box)
}

// paintStars draws motes drifting up the background, quicker while the
// song plays and nearer the beat.
func (r *root) paintStars(p *paint.Painter, box geom.Size) {
	pal := r.l.pal
	defer p.Blend(paint.BlendAdd)()
	t := float64(r.spin)
	for i := range 48 {
		seed := float64(i) * 12.9898
		x := float32(math.Mod(math.Sin(seed)*43758.5453, 1))
		if x < 0 {
			x++
		}
		speed := 0.01 + 0.03*float64(i%7)/7
		y := float32(1 - math.Mod(t*speed+float64(i)*0.137, 1))
		px := snap(x*box.W+8*sinf(t*0.3+seed), pal.pixel)
		py := snap(y*box.H, pal.pixel)
		s := float32(1.5 + float64(i%3))
		tw := 0.35 + 0.35*sinf(t*2+seed) + 0.4*r.l.pulse
		c := alpha(pal.ch[i%16], 0.25*tw)
		if pal.pixel > 1.5 {
			s = pal.pixel
			p.RRect(xyxy(px, py, px+s, py+s), 0, paint.Solid(c))
			continue
		}
		p.ShadowRRect(xyxy(px-s/2, py-s/2, px+s/2, py+s/2), s/2, paint.Solid(c), paint.Shadow{Blur: 6, Color: alpha(c, 0.6)})
	}
}

// paintTitle draws the program's name and the song's, at the top left.
func (r *root) paintTitle(p *paint.Painter, f gunim.Frame, box geom.Size) {
	pal := r.l.pal
	top := f.Safe.Top + 12
	x := float32(margin + 4)
	logo := shaped("gunim", 15, true)
	logo.Paint(p, geom.Pt(x, top), pal.accent)
	sub := shaped("MIDI PLAYER", 15, false)
	sub.Paint(p, geom.Pt(x+logo.Advance+6, top), alpha(pal.ink, 0.65))
	title := "Drop a MIDI file to play it"
	info := "Or press Open, or try the demo"
	if sc := r.l.score; sc != nil {
		title = sc.Title
		info = fmt.Sprintf("%s  ·  %d channels  ·  %.0f BPM  ·  %s", clock(sc.Length), len(sc.used()), sc.BPM, styleName(r.l.s.Style))
	}
	maxW := box.W - 2*margin - min(float32(470), box.W*0.45) - 230
	big := fit(title, 30, true, maxW)
	// The title rises into place as a song loads.
	rise := 1 - ease(r.l.fresh*2)
	big.Paint(p, geom.Pt(x, top+24+12*rise), alpha(pal.ink, 1-rise))
	fit(info, 13, false, maxW).Paint(p, geom.Pt(x+1, top+64), alpha(pal.dim, 1-rise))
}

// paintSparks sends a spark up from each key as its note starts, to the
// station of its channel, so the piano and the band are seen to play
// the same notes.
func (r *root) paintSparks(p *paint.Painter) {
	l := r.l
	if l.score == nil || l.keyY == 0 {
		return
	}
	pal := l.pal
	defer p.Blend(paint.BlendAdd)()
	const life = 0.7
	count := 0
	l.score.span(l.t-life, l.t, func(n *Note) {
		age := l.t - n.Start
		if age < 0 || age > life || count > 90 || l.score.Channels[n.Ch].Drums {
			return
		}
		st := l.station[n.Ch]
		if st.Empty() {
			return
		}
		count++
		x := float32(age / life)
		e := ease(x)
		kx := (l.keyX[n.Key][0] + l.keyX[n.Key][1]) / 2
		from := geom.Pt(kx, l.keyY)
		to := geom.Pt(st.Min.X+st.Size().W/2, st.Max.Y-st.Size().H*0.35)
		pos := geom.Pt(from.X+(to.X-from.X)*e, from.Y+(to.Y-from.Y)*e)
		// An arc out to one side on the way.
		side := float32(1)
		if n.Key%2 == 0 {
			side = -1
		}
		pos.X += side * 40 * sinf(float64(x)*math.Pi)
		c := alpha(pal.ch[n.Ch], (1-x)*0.9*float32(n.Vel)/127+0.1)
		s := (3 + 4*float32(n.Vel)/127) * (1 - 0.5*x)
		if pal.pixel > 1.5 {
			s = pal.pixel * 1.5
			pos = geom.Pt(snap(pos.X, pal.pixel), snap(pos.Y, pal.pixel))
			p.RRect(xyxy(pos.X-s/2, pos.Y-s/2, pos.X+s/2, pos.Y+s/2), 0, paint.Solid(c))
			return
		}
		p.ShadowRRect(xyxy(pos.X-s/2, pos.Y-s/2, pos.X+s/2, pos.Y+s/2), s/2, paint.Solid(c),
			paint.Shadow{Blur: 10, Color: alpha(c, 0.7)})
	})
}

// bootLines are what each style's machine says as it starts.
var bootLines = map[string][]string{
	"gm":    {"♪  THE ORCHESTRA TAKES ITS SEATS  ♪", "128 instruments  ·  one drum kit"},
	"sid":   {"**** COMMODORE 64 BASIC V2 ****", "64K RAM SYSTEM  38911 BASIC BYTES FREE", "READY."},
	"nes":   {"PRESS START", "2A03  ·  2 PULSES  ·  TRIANGLE  ·  NOISE"},
	"gb":    {"gunim®", "DOT MATRIX WITH STEREO SOUND"},
	"adlib": {"Sound Blaster detected at A220 I5 D1", "AdLib OPL2  ·  9 FM voices", "C:\\MUSIC> PLAY.EXE"},
}

// paintBoot shows the style's start-up screen over the band as the
// style changes: a line or two, as the machine would print them.
func (r *root) paintBoot(p *paint.Painter, box geom.Size) {
	if r.boot <= 0 {
		return
	}
	pal := r.l.pal
	a := min(1, r.boot*2.5)
	rc := r.band.bounds
	if rc.Empty() {
		return
	}
	cx, cy := rc.Min.X+rc.Size().W/2, rc.Min.Y+rc.Size().H/2
	lines := bootLines[r.bootStyle]
	age := 1 - r.boot
	end := p.Layer(paint.LayerOpts{Bounds: rc, Opacity: a, Clip: true, Radius: 18 * pal.round})
	p.RRect(rc, 18*pal.round, paint.Solid(alpha(pal.bgBottom, 0.82)))
	y := cy - float32(len(lines))*22
	for i, s := range lines {
		// Each line types itself out, one after another.
		shown := int(float32(len([]rune(s))) * min(1, max(0, age*4-float32(i)*0.6)))
		rs := []rune(s)
		if shown <= 0 {
			continue
		}
		size := float32(18)
		if i == 0 {
			size = 26
		}
		run := shaped(string(rs[:min(shown, len(rs))]), size, i == 0)
		if r.bootStyle == "gb" && i == 0 {
			// The Game Boy's name scrolls down into place.
			drop := 1 - ease(age*2.2)
			run.Paint(p, geom.Pt(cx-run.Advance/2, y-60*drop), pal.ink)
		} else {
			run.Paint(p, geom.Pt(cx-run.Advance/2, y), pal.ink)
		}
		y += size + 18
	}
	end()
	_ = box
}

// paintToast shows the latest message at the bottom, over the
// transport, fading.
func (r *root) paintToast(p *paint.Painter, box geom.Size) {
	if r.toast <= 0 || r.msg == "" {
		return
	}
	pal := r.l.pal
	a := min(1, r.toast*5)
	run := fit(r.msg, 14, false, box.W-120)
	w := run.Advance + 36
	y := box.H - margin - transH - 54 + 10*(1-a)
	rc := xyxy(box.W/2-w/2, y, box.W/2+w/2, y+38)
	p.ShadowRRect(rc, 19*pal.round, paint.Solid(alpha(pal.panel, a*0.95)), paint.Shadow{Blur: 18, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.5*a)})
	p.RRectStroke(rc, 19*pal.round, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.accent, a*0.7)})
	run.Paint(p, geom.Pt(rc.Min.X+18, rc.Min.Y+11), alpha(pal.ink, a))
}

// paintScan lays a console's scanlines over the window, and darkens its
// corners, as a television's glass did.
func (r *root) paintScan(p *paint.Painter, box geom.Size) {
	pal := r.l.pal
	if pal.scan < 0.01 {
		return
	}
	c := alpha(color.NRGBA{A: 255}, pal.scan)
	for y := float32(0); y < box.H; y += 3 {
		p.RRect(xyxy(0, y, box.W, y+1), 0, paint.Solid(c))
	}
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(box.W/2, box.H/2),
		To: geom.Pt(box.W*1.05, box.H*1.05), Radial: true, Start: color.NRGBA{}, End: alpha(color.NRGBA{A: 255}, pal.scan*2.2),
		Stops: []paint.Stop{{At: 0.6, Color: color.NRGBA{}}}}})
}

// paintDrop draws a drag of files over the window: the window dims and
// blurs, and a disc spins in its middle, ready to take them.
func (r *root) paintDrop(p *paint.Painter, box geom.Size) {
	d := ease(r.drag)
	if d <= 0.001 && r.dropped <= 0 {
		return
	}
	pal := r.l.pal
	full := geom.Rect{Max: box.Point()}
	cx, cy := box.W/2, box.H/2
	if d > 0.001 {
		end := p.Layer(paint.LayerOpts{Bounds: full, Opacity: d, Backdrop: 10})
		p.RRect(full, 0, paint.Solid(alpha(pal.bgBottom, 0.7)))
		rad := float32(130) + 8*sinf(float64(r.spin)*3)
		c := pal.accent
		if !r.dragOK {
			c = rgb(0xff5c5c)
		}
		// A record spinning, its grooves, and its label.
		p.ShadowRRect(xyxy(cx-rad, cy-rad, cx+rad, cy+rad), rad, paint.Solid(rgb(0x111111)),
			paint.Shadow{Blur: 60, Color: alpha(c, 0.55)})
		for g := float32(0.45); g < 0.98; g += 0.07 {
			rr := rad * g
			p.RRectStroke(xyxy(cx-rr, cy-rr, cx+rr, cy+rr), rr, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.white, 0.08)})
		}
		lab := rad * 0.36
		p.RRect(xyxy(cx-lab, cy-lab, cx+lab, cy+lab), lab, paint.Solid(c))
		a := float64(r.spin) * 2.5
		for i := range 3 {
			ang := a + float64(i)*2*math.Pi/3
			hx, hy := cx+cosf(ang)*rad*0.7, cy+sinf(ang)*rad*0.7
			p.RRect(xyxy(hx-5, hy-5, hx+5, hy+5), 5, paint.Solid(alpha(pal.white, 0.35)))
		}
		p.RRect(xyxy(cx-6, cy-6, cx+6, cy+6), 6, paint.Solid(rgb(0x111111)))
		msg := "Drop to play"
		if r.dragN > 1 {
			msg = fmt.Sprintf("Drop to play %d files", r.dragN)
		}
		if !r.dragOK {
			msg = "Those are not MIDI files"
		}
		run := shaped(msg, 26, true)
		run.Paint(p, geom.Pt(cx-run.Advance/2, cy+rad+28), pal.ink)
		sub := shaped(".mid  ·  .midi  ·  .kar  ·  or a folder of them", 13, false)
		sub.Paint(p, geom.Pt(cx-sub.Advance/2, cy+rad+66), pal.dim)
		end()
	}
	if r.dropped > 0 {
		// The drop lands: a ring flies out from the middle.
		x := 1 - r.dropped
		rad := 130 + x*max(box.W, box.H)*0.7
		defer p.Blend(paint.BlendAdd)()
		p.RRectStroke(xyxy(cx-rad, cy-rad, cx+rad, cy+rad), rad, paint.Fill{}, paint.Stroke{Width: 6 * r.dropped, Color: alpha(pal.accent, r.dropped)})
	}
}

// takes reports which of paths the player takes, and whether any.
func takes(paths []string) (n int) {
	for _, p := range paths {
		if isMIDI(p) || filepath.Ext(p) == "" {
			n++
		}
	}
	return n
}

// Handle takes drags and drops of files over the window.
func (r *root) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.DragOver:
		f, ok := e.Data.(input.Files)
		if !ok {
			return false
		}
		r.dragN = takes(f.Paths)
		r.dragOK = r.dragN > 0 || len(f.Paths) == 0
		r.drag = 1
		u.Invalidate()
		return true
	case input.DragLeave:
		r.drag = 0
		u.Invalidate()
		return true
	case input.Drop:
		r.drag = 0
		paths := e.Paths
		if f, ok := e.Data.(input.Files); ok && len(paths) == 0 {
			paths = f.Paths
		}
		if len(paths) == 0 {
			u.Invalidate()
			return false
		}
		r.dropped = 1
		u.Send(r, FilesDropped{Paths: paths})
		u.Invalidate()
		return true
	}
	return false
}

// CatchKey plays the keys nothing focused took: Space plays and pauses,
// the arrows move five seconds, N and P skip, 1 to 5 pick a style, M
// shows the mixer or the stage, L shows the playlist and O opens.
func (r *root) CatchKey(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	s := r.l.s
	switch {
	case k.Key == input.KeySpace:
		u.Send(r, PlayToggled{})
	case k.Key == input.KeyRight:
		u.Send(r, Sought{Time: r.l.t + 5})
	case k.Key == input.KeyLeft:
		u.Send(r, Sought{Time: r.l.t - 5})
	case k.Char == 'n':
		u.Send(r, Skipped{By: 1})
	case k.Char == 'p':
		u.Send(r, Skipped{By: -1})
	case k.Char == 'o' || k.Char == 'O':
		u.Send(r, OpenAsked{})
	case k.Char == 'm':
		r.page = 1 - r.page
		u.Invalidate()
	case k.Char == 'l':
		r.list.open = !r.list.open
		r.list.closed = !r.list.open
		u.Invalidate()
	case k.Char >= '1' && k.Char <= '5':
		u.Send(r, StyleChosen{Style: styleOrder[k.Char-'1']})
	case k.Char == '+' || k.Char == '=':
		u.Send(r, SpeedSet{Speed: s.Speed + 0.1})
	case k.Char == '-':
		u.Send(r, SpeedSet{Speed: s.Speed - 0.1})
	default:
		return false
	}
	return true
}

// startPage is the page the window opens on: 0 the stage, 1 the
// mixer.
var startPage int

// styleOrder is the order of the styles on the deck.
var styleOrder = []string{"gm", "sid", "nes", "gb", "adlib"}

// styleName names a style as the deck does.
func styleName(s string) string {
	switch s {
	case "sid":
		return "Commodore 64"
	case "nes":
		return "NES"
	case "gb":
		return "Game Boy"
	case "adlib":
		return "AdLib"
	}
	return "General MIDI"
}

// shaped is text shaped at size, kept for the next frame.
func shaped(s string, size float32, bold bool) text.Run { return audioui.Shaped(s, size, bold, false) }

// fit is s shaped at size, cut with an ellipsis to fit width w.
func fit(s string, size float32, bold bool, w float32) text.Run {
	run := shaped(s, size, bold)
	if run.Advance <= w || w <= 0 {
		return run
	}
	rs := []rune(s)
	lo, hi := 0, len(rs)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if shaped(string(rs[:mid])+"…", size, bold).Advance <= w {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return shaped(strings.TrimSpace(string(rs[:lo]))+"…", size, bold)
}

// clock writes secs as minutes and seconds.
func clock(secs float64) string {
	s := int(max(secs, 0))
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// tabs are the two pages' tabs: the stage and the mixer. The pill
// slides to the page shown.
type tabs struct {
	r    *root
	over int
	size geom.Size
}

func (t *tabs) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	return c.Max
}

func (t *tabs) tabAt(pos geom.Point) int {
	if pos.Y < 0 || pos.Y > t.size.H || pos.X < 0 || pos.X > t.size.W {
		return -1
	}
	return min(int(pos.X/(t.size.W/2)), 1)
}

func (t *tabs) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if o := t.tabAt(e.Pos); o != t.over {
			t.over = o
			u.Invalidate()
		}
		return false
	case input.PointerLeave:
		t.over = -1
		u.Invalidate()
	case input.PointerDown:
		if i := t.tabAt(e.Pos); i >= 0 && e.Button == input.ButtonPrimary {
			t.r.page = i
			u.Invalidate()
			return true
		}
		return false
	default:
		return false
	}
	return true
}

func (t *tabs) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	pal := t.r.l.pal
	rad := box.H / 2 * min(1, pal.round*3)
	full := geom.Rect{Max: box.Point()}
	p.RRect(full, rad, paint.Solid(alpha(darken(pal.panel, 0.3), 0.9)))
	p.RRectStroke(full, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	half := box.W / 2
	x := half * ease(t.r.pg)
	p.ShadowRRect(xyxy(x+3, 3, x+half-3, box.H-3), rad-3, paint.Solid(pal.accent),
		paint.Shadow{Blur: 10, Color: alpha(pal.accent, 0.4)})
	for i, label := range []string{"Stage", "Mixer"} {
		ic := icon.Music
		if i == 1 {
			ic = icon.SlidersVertical
		}
		on := i == t.r.page
		ink := pal.ink
		if on {
			ink = darken(pal.accent, 0.8)
		} else if i == t.over {
			ink = lighten(pal.ink, 0.2)
		}
		run := shaped(label, 13, true)
		w := 16 + 6 + run.Advance
		x0 := float32(i)*half + (half-w)/2
		p.Mask(icon.Stroke{Icon: ic, Width: 2, Progress: 1}, xyxy(x0, box.H/2-8, x0+16, box.H/2+8), pal.tone(ink))
		run.Paint(p, geom.Pt(x0+22, (box.H-run.Height())/2), ink)
	}
}
