package main

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// pen is what an instrument is drawn with: the palette, the channel's
// colour, the time heard, what the channel plays, and its instrument.
type pen struct {
	pal   palette
	col   color.NRGBA
	t     float64
	live  *chanLive
	info  ChannelInfo
	prog  int
	drums bool
}

// drawInstrument draws the channel's instrument in r, playing what it
// plays: a kit for the drums, and for the others the instrument of the
// family its program is in.
func drawInstrument(p *paint.Painter, pn pen, r geom.Rect) {
	if pn.drums {
		drawKit(p, pn, r)
		return
	}
	switch pn.prog / 8 {
	case 0:
		drawPiano(p, pn, r)
	case 1:
		if pn.prog == 14 {
			drawTubes(p, pn, r)
		} else {
			drawBars(p, pn, r)
		}
	case 2:
		drawPipes(p, pn, r)
	case 3:
		drawGuitar(p, pn, r, pn.prog == 29 || pn.prog == 30)
	case 4:
		drawSpeaker(p, pn, r)
	case 5:
		switch pn.prog {
		case 45, 46:
			drawHarp(p, pn, r)
		case 47:
			drawTimpani(p, pn, r)
		default:
			drawBow(p, pn, r, 1)
		}
	case 6:
		switch {
		case pn.prog >= 52 && pn.prog <= 54:
			drawChoir(p, pn, r)
		case pn.prog == 55:
			drawBurst(p, pn, r)
		default:
			drawBow(p, pn, r, 3)
		}
	case 7:
		drawBell(p, pn, r)
	case 8:
		drawSax(p, pn, r)
	case 9:
		drawFlute(p, pn, r)
	case 10:
		drawScope(p, pn, r)
	case 11:
		drawAurora(p, pn, r)
	case 12:
		drawSparkle(p, pn, r)
	case 13:
		switch pn.prog {
		case 108:
			drawKalimba(p, pn, r)
		case 109:
			drawBag(p, pn, r)
		case 110:
			drawBow(p, pn, r, 1)
		default:
			drawGuitar(p, pn, r, false)
		}
	case 14:
		drawPad(p, pn, r)
	default:
		drawSFX(p, pn, r)
	}
}

// The drawing helpers: each takes the palette's corners and fat pixels.

func (pn pen) rect(p *paint.Painter, r geom.Rect, rad float32, c color.NRGBA) {
	if px := pn.pal.pixel; px > 1.5 {
		r = xyxy(snap(r.Min.X, px), snap(r.Min.Y, px), snap(r.Max.X, px), snap(r.Max.Y, px))
		if r.Size().W < px {
			r.Max.X = r.Min.X + px
		}
		if r.Size().H < px {
			r.Max.Y = r.Min.Y + px
		}
	}
	p.RRect(r, rad*pn.pal.round, paint.Solid(pn.pal.tone(c)))
}

func (pn pen) circle(p *paint.Painter, cx, cy, rad float32, c color.NRGBA) {
	pn.rect(p, xyxy(cx-rad, cy-rad, cx+rad, cy+rad), rad/max(pn.pal.round, 0.01)*min(1, pn.pal.round*3), c)
}

func (pn pen) ring(p *paint.Painter, cx, cy, rad, w float32, c color.NRGBA) {
	rr := rad * min(1, pn.pal.round*3)
	p.RRectStroke(xyxy(cx-rad, cy-rad, cx+rad, cy+rad), rr, paint.Fill{}, paint.Stroke{Width: w, Color: pn.pal.tone(c)})
}

// glow adds a soft light at cx, cy.
func (pn pen) glow(p *paint.Painter, cx, cy, rad float32, c color.NRGBA) {
	if rad <= 0.5 || c.A == 0 {
		return
	}
	c = pn.pal.tone(c)
	defer p.Blend(paint.BlendAdd)()
	if pn.pal.pixel > 1.5 {
		pn.rect(p, xyxy(cx-rad*0.5, cy-rad*0.5, cx+rad*0.5, cy+rad*0.5), 0, c)
		return
	}
	p.ShadowRRect(xyxy(cx-rad*0.4, cy-rad*0.4, cx+rad*0.4, cy+rad*0.4), rad*0.4, paint.Solid(c),
		paint.Shadow{Blur: rad, Color: c})
}

// line draws a line from a to b, w wide.
func (pn pen) line(p *paint.Painter, a, b geom.Point, w float32, c color.NRGBA) {
	c = pn.pal.tone(c)
	dx, dy := b.X-a.X, b.Y-a.Y
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l < 0.01 {
		return
	}
	mid := geom.Pt((a.X+b.X)/2, (a.Y+b.Y)/2)
	end := p.Push(paint.Rotate(float32(math.Atan2(float64(dy), float64(dx))), mid))
	p.RRect(xyxy(mid.X-l/2, mid.Y-w/2, mid.X+l/2, mid.Y+w/2), w/2, paint.Solid(c))
	end()
}

// pos returns where key lies in the channel's range, from 0 to 1.
func (pn pen) pos(key uint8) float32 {
	lo, hi := float32(pn.info.Low), float32(pn.info.High)
	if hi-lo < 12 {
		m := (lo + hi) / 2
		lo, hi = m-6, m+6
	}
	return min(max((float32(key)-lo)/(hi-lo), 0), 1)
}

// strike returns how hard a note struck age ago is still felt: its
// velocity, falling away by fall a second, and once it ends, fast.
func strike(n liveNote, fall float64) float32 {
	s := n.vel * float32(math.Exp(-n.age*fall))
	if n.left < 0 {
		s *= float32(math.Exp(n.left * 10))
	}
	return s
}

// held returns how much a note sounds now: its velocity while held,
// fading once let go.
func held(n liveNote) float32 {
	if n.left >= 0 {
		return n.vel
	}
	return n.vel * float32(math.Exp(n.left*8))
}

// drawPiano draws a keyboard over the channel's range, its keys going
// down as they play, and the hammers over them jumping.
func drawPiano(p *paint.Painter, pn pen, r geom.Rect) {
	lo := int(pn.info.Low) - int(pn.info.Low)%12
	hi := int(pn.info.High) + 11 - int(pn.info.High)%12
	if hi-lo < 24 {
		hi = lo + 23
	}
	hi = min(hi, lo+47)
	var down [128]float32
	for _, n := range pn.live.notes {
		down[n.key] = max(down[n.key], held(n))
	}
	keys := xyxy(r.Min.X, r.Min.Y+r.Size().H*0.38, r.Max.X, r.Max.Y)
	xs := keyLayout(lo, hi, keys.Min.X, keys.Max.X)
	kh := keys.Size().H
	// The hammers, above the keys they strike.
	for _, n := range pn.live.notes {
		x := (xs[n.key][0] + xs[n.key][1]) / 2
		jump := strike(n, 9) * r.Size().H * 0.3
		y := keys.Min.Y - 8 - jump
		pn.rect(p, xyxy(x-2, y, x+2, keys.Min.Y-4), 2, alpha(pn.pal.ink, 0.5))
		pn.circle(p, x, y, 4, lighten(pn.col, 0.3))
		pn.glow(p, x, y, 14*strike(n, 6), alpha(pn.col, 0.8))
	}
	for k := lo; k <= hi; k++ {
		if black(k) {
			continue
		}
		d := down[k]
		c := mix(rgb(0xf4f2ea), pn.col, d*0.8)
		pn.rect(p, xyxy(xs[k][0]+0.5, keys.Min.Y+d*3, xs[k][1]-0.5, keys.Max.Y), 3, c)
	}
	for k := lo; k <= hi; k++ {
		if !black(k) {
			continue
		}
		d := down[k]
		c := mix(rgb(0x1a1a22), pn.col, d)
		pn.rect(p, xyxy(xs[k][0], keys.Min.Y+d*2, xs[k][1], keys.Min.Y+kh*0.6+d*2), 2, c)
	}
}

// black reports whether key is a black key.
func black(key int) bool {
	switch key % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// keyLayout returns where each key from lo to hi lies between x0 and x1,
// its left and right, as a piano's: the white keys side by side, and
// each black key over the gap between two.
func keyLayout(lo, hi int, x0, x1 float32) [128][2]float32 {
	var xs [128][2]float32
	whites := 0
	for k := lo; k <= hi; k++ {
		if !black(k) {
			whites++
		}
	}
	w := (x1 - x0) / float32(max(whites, 1))
	x := x0
	for k := lo; k <= hi && k < 128; k++ {
		if k < 0 {
			continue
		}
		if black(k) {
			xs[k] = [2]float32{x - w*0.32, x + w*0.32}
			continue
		}
		xs[k] = [2]float32{x, x + w}
		x += w
	}
	return xs
}

// drawBars draws a mallet instrument's bars, a bar for each of the
// twelve notes, short to long; a bar struck jumps and rings out in
// circles.
func drawBars(p *paint.Painter, pn pen, r geom.Rect) {
	n := 12
	w := r.Size().W / float32(n)
	var hit [12]float32
	for _, nt := range pn.live.notes {
		hit[nt.key%12] = max(hit[nt.key%12], strike(nt, 5))
		if nt.age < 0.8 {
			// A ring, flying out from the bar.
			i := int(nt.key % 12)
			cx := r.Min.X + (float32(i)+0.5)*w
			cy := r.Min.Y + r.Size().H*0.5
			rad := 6 + float32(nt.age)*60
			pn.ring(p, cx, cy, rad, 2, alpha(pn.col, float32(1-nt.age/0.8)*0.6))
		}
	}
	for i := range n {
		h := r.Size().H * (0.95 - 0.45*float32(i)/float32(n))
		cx := r.Min.X + (float32(i)+0.5)*w
		top := r.Min.Y + (r.Size().H-h)/2 - hit[i]*8
		c := mix(darken(pn.col, 0.45), lighten(pn.col, 0.4), hit[i])
		pn.rect(p, xyxy(cx-w*0.38, top, cx+w*0.38, top+h), 4, c)
		pn.circle(p, cx, top+h*0.18, 2, alpha(pn.pal.bgBottom, 0.6))
		pn.circle(p, cx, top+h*0.82, 2, alpha(pn.pal.bgBottom, 0.6))
		pn.glow(p, cx, top+h/2, 26*hit[i], alpha(pn.col, 0.8))
	}
}

// drawTubes draws tubular bells hanging from a rail, swinging as they
// are struck.
func drawTubes(p *paint.Painter, pn pen, r geom.Rect) {
	n := 8
	w := r.Size().W / float32(n)
	var hit [8]float32
	var age [8]float64
	for _, nt := range pn.live.notes {
		i := int(pn.pos(nt.key) * float32(n-1))
		if s := strike(nt, 1.5); s > hit[i] {
			hit[i], age[i] = s, nt.age
		}
	}
	pn.rect(p, xyxy(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+5), 2, alpha(pn.pal.ink, 0.5))
	for i := range n {
		cx := r.Min.X + (float32(i)+0.5)*w
		h := r.Size().H * (0.95 - 0.5*float32(i)/float32(n))
		swing := hit[i] * 0.25 * sinf(age[i]*9)
		top := geom.Pt(cx, r.Min.Y+4)
		end := p.Push(paint.Rotate(swing, top))
		c := mix(rgb(0xc9c3b0), pn.col, 0.3+0.7*hit[i])
		pn.rect(p, xyxy(cx-w*0.22, top.Y+6, cx+w*0.22, top.Y+6+h), 3, c)
		pn.rect(p, xyxy(cx-w*0.28, top.Y+6, cx+w*0.28, top.Y+12), 2, lighten(c, 0.3))
		pn.glow(p, cx, top.Y+6+h*0.3, 22*hit[i], alpha(pn.col, 0.9))
		end()
	}
}

// drawPipes draws an organ's pipes, lit from inside as they sound, air
// rising from their tops.
func drawPipes(p *paint.Painter, pn pen, r geom.Rect) {
	n := 12
	w := r.Size().W / float32(n)
	var on [12]float32
	for _, nt := range pn.live.notes {
		on[nt.key%12] = max(on[nt.key%12], held(nt))
	}
	for i := range n {
		// The pipes stand as an organ's front does: tall in the middle.
		d := math.Abs(float64(i)-5.5) / 5.5
		h := r.Size().H * float32(0.92-0.5*d)
		cx := r.Min.X + (float32(i)+0.5)*w
		top := r.Max.Y - h
		body := mix(rgb(0xb8b2a0), pn.col, 0.25+0.6*on[i])
		pn.rect(p, xyxy(cx-w*0.36, top, cx+w*0.36, r.Max.Y), 3, body)
		pn.rect(p, xyxy(cx-w*0.2, top+h*0.65, cx+w*0.2, top+h*0.72), 2, alpha(pn.pal.bgBottom, 0.7))
		if on[i] > 0.05 {
			pn.glow(p, cx, top+h*0.4, 30*on[i], alpha(pn.col, 0.7))
			// Puffs of air, rising and fading.
			for j := range 3 {
				ph := math.Mod(pn.t*1.6+float64(j)/3+float64(i)*0.13, 1)
				y := top - float32(ph)*r.Size().H*0.4
				pn.circle(p, cx+4*sinf(pn.t*3+float64(j+i)), y, 2+3*float32(ph), alpha(pn.pal.ink, on[i]*0.5*float32(1-ph)))
			}
		}
	}
}

// drawGuitar draws a guitar's six strings over its sound hole, each
// shaking as it is plucked; overdriven, the strings burn and the hole
// glows like an amp.
func drawGuitar(p *paint.Painter, pn pen, r geom.Rect, driven bool) {
	cx, cy := r.Min.X+r.Size().W*0.5, r.Min.Y+r.Size().H*0.5
	rad := min(r.Size().H*0.42, r.Size().W*0.22)
	hole := darken(pn.col, 0.7)
	if driven {
		hole = mix(rgb(0x2a0a00), rgb(0xff5a1f), pn.live.level*0.6)
	}
	pn.circle(p, cx, cy, rad+5, alpha(lighten(pn.col, 0.2), 0.35))
	pn.circle(p, cx, cy, rad, hole)
	if driven {
		pn.glow(p, cx, cy, rad*2*pn.live.level, alpha(rgb(0xff6a1f), 0.8))
	}
	var amp [6]float32
	var age [6]float64
	for _, n := range pn.live.notes {
		i := 5 - int(pn.pos(n.key)*5.99)
		if s := strike(n, 3.5); s > amp[i] {
			amp[i], age[i] = s, n.age
		}
	}
	gap := r.Size().H * 0.8 / 6
	for s := range 6 {
		y := cy - gap*2.5 + float32(s)*gap
		c := mix(rgb(0xd8d0c0), pn.col, amp[s])
		if driven && amp[s] > 0.1 {
			c = mix(c, rgb(0xffb070), amp[s])
		}
		w := 1 + float32(s)*0.35
		if amp[s] < 0.02 {
			pn.rect(p, xyxy(r.Min.X, y-w/2, r.Max.X, y+w/2), w/2, c)
			continue
		}
		// The string as dots along its shake, a half sine across it.
		const steps = 36
		for k := range steps {
			x := float32(k) / (steps - 1)
			shake := amp[s] * 7 * sinf(float64(x)*math.Pi) * sinf(age[s]*70+float64(s))
			px := r.Min.X + x*r.Size().W
			pn.rect(p, xyxy(px-r.Size().W/steps/2-0.5, y+shake-w/2, px+r.Size().W/steps/2+0.5, y+shake+w/2), w/2, c)
		}
		pn.glow(p, cx, y, 24*amp[s], alpha(pn.col, 0.5))
	}
}

// drawSpeaker draws a bass's speaker, its cone thumping with each note
// and rings going out from it.
func drawSpeaker(p *paint.Painter, pn pen, r geom.Rect) {
	cx, cy := r.Min.X+r.Size().W*0.5, r.Min.Y+r.Size().H*0.5
	rad := min(r.Size().H, r.Size().W) * 0.46
	thump := pn.live.level*0.06 + pn.live.hit*0.08
	for _, n := range pn.live.notes {
		if n.age < 0.5 {
			rr := rad * (1 + float32(n.age)*1.4)
			pn.ring(p, cx, cy, rr, 2, alpha(pn.col, float32(1-n.age/0.5)*0.5*n.vel))
		}
	}
	pn.circle(p, cx, cy, rad, rgb(0x15151c))
	pn.ring(p, cx, cy, rad*(0.93+thump), 4, alpha(pn.col, 0.7))
	pn.circle(p, cx, cy, rad*(0.82+thump), rgb(0x22222c))
	for i := 1; i <= 3; i++ {
		pn.ring(p, cx, cy, rad*(0.82-float32(i)*0.15)*(1+thump), 1, alpha(pn.pal.ink, 0.12))
	}
	cap := rad * (0.28 + thump*1.5)
	pn.circle(p, cx, cy, cap, mix(rgb(0x30303c), pn.col, 0.3+0.7*pn.live.hit))
	pn.glow(p, cx, cy, rad*1.3*pn.live.hit, alpha(pn.col, 0.6))
	// Four screws round the rim.
	for i := range 4 {
		a := float64(i)*math.Pi/2 + math.Pi/4
		pn.circle(p, cx+cosf(a)*rad*0.97, cy+sinf(a)*rad*0.97, 2.5, alpha(pn.pal.ink, 0.4))
	}
}

// drawBow draws strings under a bow, the bow drawn back and forth while
// notes sound, and the strings shaking; sections draws as many as play.
func drawBow(p *paint.Painter, pn pen, r geom.Rect, sections int) {
	w := r.Size().W / float32(sections)
	sound := float32(0)
	for _, n := range pn.live.notes {
		sound = max(sound, held(n))
	}
	for sct := range sections {
		x0 := r.Min.X + float32(sct)*w
		cx := x0 + w/2
		// Four strings standing up.
		for s := range 4 {
			x := cx + (float32(s)-1.5)*min(w*0.12, 10)
			shake := sound * 1.5 * sinf(pn.t*90+float64(s*3+sct))
			pn.rect(p, xyxy(x-0.8+shake, r.Min.Y, x+0.8+shake, r.Max.Y), 1, mix(rgb(0xd8d0c0), pn.col, sound))
		}
		// The bow across them, sliding while they sound.
		slide := float32(0)
		if sound > 0.02 {
			slide = sinf(pn.t*2.2+float64(sct)*1.3) * w * 0.25
		}
		by := r.Min.Y + r.Size().H*0.6
		tilt := float32(-0.18)
		end := p.Push(paint.Rotate(tilt, geom.Pt(cx, by)))
		pn.rect(p, xyxy(cx-w*0.48+slide, by-1.5, cx+w*0.48+slide, by+1.5), 1.5, lighten(pn.col, 0.3))
		pn.rect(p, xyxy(cx-w*0.48+slide, by-6, cx-w*0.48+slide+3, by+1), 1, rgb(0x6b4a2b))
		end()
		pn.glow(p, cx, by, 30*sound, alpha(pn.col, 0.6))
	}
}

// drawHarp draws a harp's strings in its frame, each shaking as it is
// plucked.
func drawHarp(p *paint.Painter, pn pen, r geom.Rect) {
	n := 14
	var amp [14]float32
	var age [14]float64
	for _, nt := range pn.live.notes {
		i := int(pn.pos(nt.key) * float32(n-1))
		if s := strike(nt, 2.5); s > amp[i] {
			amp[i], age[i] = s, nt.age
		}
	}
	gold := mix(rgb(0xd9a441), pn.col, 0.3)
	pn.rect(p, xyxy(r.Min.X, r.Max.Y-6, r.Max.X, r.Max.Y), 3, gold)
	pn.rect(p, xyxy(r.Min.X, r.Min.Y, r.Min.X+6, r.Max.Y), 3, gold)
	for i := range n {
		x := r.Min.X + 12 + float32(i)*(r.Size().W-18)/float32(n-1)
		// The neck falls from left to right, so the strings shorten.
		top := r.Min.Y + r.Size().H*0.6*float32(i)/float32(n)
		shake := amp[i] * 4 * sinf(age[i]*60)
		c := mix(alpha(pn.pal.ink, 0.6), pn.col, amp[i])
		if i%7 == 0 {
			c = mix(rgb(0xd04040), pn.col, amp[i])
		}
		mid := (top + r.Max.Y) / 2
		pn.line(p, geom.Pt(x, top), geom.Pt(x+shake, mid), 1.2, c)
		pn.line(p, geom.Pt(x+shake, mid), geom.Pt(x, r.Max.Y-6), 1.2, c)
		pn.glow(p, x, mid, 18*amp[i], alpha(pn.col, 0.7))
	}
	pn.line(p, geom.Pt(r.Min.X, r.Min.Y), geom.Pt(r.Max.X, r.Min.Y+r.Size().H*0.6), 5, gold)
}

// drawTimpani draws a kettledrum: a copper bowl on three legs, its head
// flexing as it is struck, and rings going out across it.
func drawTimpani(p *paint.Painter, pn pen, r geom.Rect) {
	cx := r.Min.X + r.Size().W/2
	w := min(r.Size().W*0.7, r.Size().H*1.5)
	top := r.Min.Y + r.Size().H*0.28
	hit := pn.live.hit
	copper := mix(rgb(0xb8642b), pn.col, 0.2)
	bowlH := min(w*0.55, r.Max.Y-top-6)
	for _, x := range []float32{-0.36, 0, 0.36} {
		pn.line(p, geom.Pt(cx+x*w, top+bowlH*0.6), geom.Pt(cx+x*w*1.25, r.Max.Y), 3, alpha(pn.pal.ink, 0.4))
	}
	// The bowl: round below, cut straight across at the head.
	clip := p.Layer(paint.LayerOpts{Bounds: xyxy(cx-w/2-1, top, cx+w/2+1, top+bowlH+1), Opacity: 1, Clip: true})
	pn.rect(p, xyxy(cx-w/2, top-bowlH, cx+w/2, top+bowlH), bowlH, copper)
	clip()
	pn.rect(p, xyxy(cx-w*0.3, top+bowlH*0.25, cx-w*0.05, top+bowlH*0.4), 3, alpha(lighten(copper, 0.5), 0.5))
	head := 8 + hit*5
	pn.rect(p, xyxy(cx-w/2-3, top-head/2, cx+w/2+3, top+head/2), head/2, mix(rgb(0xe8e0d0), pn.col, hit*0.6))
	pn.glow(p, cx, top, w*0.6*hit, alpha(pn.col, 0.6))
	for _, n := range pn.live.notes {
		if n.age < 0.6 {
			rw := w * (0.5 + float32(n.age)*1.2)
			x := float32(1 - n.age/0.6)
			p.RRectStroke(xyxy(cx-rw/2, top-rw*0.1, cx+rw/2, top+rw*0.1), rw*0.1, paint.Fill{},
				paint.Stroke{Width: 2, Color: pn.pal.tone(alpha(pn.col, x*0.6))})
		}
	}
}

// drawChoir draws a row of singers, their mouths opening with the notes
// they sing, the highest singer singing the highest note.
func drawChoir(p *paint.Painter, pn pen, r geom.Rect) {
	n := 5
	var open [5]float32
	for _, nt := range pn.live.notes {
		i := int(pn.pos(nt.key) * float32(n-1))
		open[i] = max(open[i], held(nt))
	}
	w := r.Size().W / float32(n)
	rad := min(w*0.36, r.Size().H*0.3)
	for i := range n {
		cx := r.Min.X + (float32(i)+0.5)*w
		sway := 3 * open[i] * sinf(pn.t*2+float64(i))
		cy := r.Min.Y + r.Size().H*0.42 + float32(i%2)*4 + sway
		// A robe, then the head, the eyes shut, and the mouth.
		pn.rect(p, xyxy(cx-rad*1.05, cy+rad*0.8, cx+rad*1.05, r.Max.Y+10), rad*0.8, mix(darken(pn.col, 0.4), pn.col, open[i]*0.5))
		pn.circle(p, cx, cy, rad, mix(rgb(0xf0d5b8), lighten(pn.col, 0.6), 0.25))
		pn.rect(p, xyxy(cx-rad*0.5, cy-rad*0.15, cx-rad*0.2, cy-rad*0.08), 1, rgb(0x2a2030))
		pn.rect(p, xyxy(cx+rad*0.2, cy-rad*0.15, cx+rad*0.5, cy-rad*0.08), 1, rgb(0x2a2030))
		mh := rad * (0.08 + 0.5*open[i])
		mw := rad * (0.3 + 0.1*open[i])
		pn.rect(p, xyxy(cx-mw/2, cy+rad*0.3, cx+mw/2, cy+rad*0.3+mh), mw/2, rgb(0x5a1a2a))
		if open[i] > 0.1 {
			// A note rises from each singer singing.
			ph := float32(math.Mod(pn.t*0.8+float64(i)*0.3, 1))
			g := shaped("♪", 14, true)
			g.Paint(p, geom.Pt(cx+rad*0.6, cy-rad-ph*r.Size().H*0.4), alpha(pn.col, open[i]*(1-ph)))
		}
	}
}

// drawBurst draws an orchestra hit: rays bursting from the middle with
// each hit.
func drawBurst(p *paint.Painter, pn pen, r geom.Rect) {
	cx, cy := r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2
	hit := pn.live.hit
	reach := min(r.Size().W, r.Size().H) * 0.5
	for i := range 16 {
		a := float64(i)/16*2*math.Pi + float64(pn.live.struck)*0.4
		l := reach * (0.3 + 0.7*hit) * (0.7 + 0.3*float32(i%2))
		pn.line(p, geom.Pt(cx+cosf(a)*l*0.25, cy+sinf(a)*l*0.25), geom.Pt(cx+cosf(a)*l, cy+sinf(a)*l), 3, alpha(pn.col, 0.3+0.7*hit))
	}
	pn.circle(p, cx, cy, reach*0.22*(1+hit), lighten(pn.col, 0.4*hit))
	pn.glow(p, cx, cy, reach*1.5*hit, alpha(pn.col, 0.8))
}

// drawBell draws a horn's bell, rings of sound going out of it with each
// note, the tubing glinting as it plays.
func drawBell(p *paint.Painter, pn pen, r geom.Rect) {
	cy := r.Min.Y + r.Size().H*0.5
	bx := r.Max.X - r.Size().H*0.45
	rad := r.Size().H * 0.42
	brass := mix(rgb(0xd9a441), pn.col, 0.25)
	for _, n := range pn.live.notes {
		for k := range 3 {
			a := n.age - float64(k)*0.12
			if a < 0 || a > 0.7 {
				continue
			}
			x := float32(a / 0.7)
			rr := rad * (0.6 + x*1.5)
			pn.ring(p, bx, cy, rr, 2.5, alpha(pn.col, (1-x)*0.7*held(n)))
		}
	}
	// The tubing, and the valves, pressed by the notes.
	tube := xyxy(r.Min.X+4, cy-5, bx, cy+5)
	pn.rect(p, tube, 5, darken(brass, 0.15))
	for i := range 3 {
		vx := r.Min.X + r.Size().W*0.2 + float32(i)*14
		down := float32(0)
		for _, n := range pn.live.notes {
			if int(n.key)%3 == i && n.left > 0 {
				down = 5
			}
		}
		pn.rect(p, xyxy(vx-4, cy-18+down, vx+4, cy-5), 2, lighten(brass, 0.2))
		pn.rect(p, xyxy(vx-5, cy-21+down, vx+5, cy-17+down), 2, rgb(0xf0e8d8))
	}
	pn.circle(p, bx, cy, rad, brass)
	pn.circle(p, bx, cy, rad*0.72, darken(brass, 0.45))
	pn.glow(p, bx, cy, rad*1.4*pn.live.level, alpha(pn.col, 0.7))
}

// drawSax draws a saxophone: its neck, its body down to the bend, and
// its bell turning up, its keys lit as the notes play, and notes
// floating up out of the bell.
func drawSax(p *paint.Painter, pn pen, r geom.Rect) {
	brass := mix(rgb(0xd9a441), pn.col, 0.2)
	at := func(fx, fy float32) geom.Point {
		// The sax stands in a box twice as tall as wide, in the middle.
		w := min(r.Size().W*0.8, r.Size().H*0.9)
		return geom.Pt(r.Min.X+r.Size().W/2+(fx-0.5)*w, r.Min.Y+fy*r.Size().H)
	}
	path := []geom.Point{at(0.18, 0.02), at(0.36, 0.06), at(0.44, 0.16), at(0.44, 0.78), at(0.56, 0.95), at(0.7, 0.86), at(0.74, 0.5)}
	for i := 1; i < len(path); i++ {
		w := 4 + 10*float32(i)/float32(len(path)-1)
		pn.line(p, path[i-1], path[i], w, darken(brass, 0.1))
		pn.circle(p, path[i].X, path[i].Y, w/2, darken(brass, 0.1))
	}
	bell := path[len(path)-1]
	pn.rect(p, xyxy(bell.X-13, bell.Y-6, bell.X+13, bell.Y+4), 5, brass)
	pn.rect(p, xyxy(bell.X-10, bell.Y-5, bell.X+10, bell.Y-1), 2, darken(brass, 0.5))
	// The keys, down the body: the lower the note, the more are closed.
	n := 6
	closed := 0
	sound := float32(0)
	for _, nt := range pn.live.notes {
		if h := held(nt); h > sound {
			sound = h
			closed = n - int(pn.pos(nt.key)*float32(n))
		}
	}
	for i := range n {
		y := path[2].Y + (path[3].Y-path[2].Y)*(float32(i)+0.5)/float32(n)
		c := rgb(0xf0e8d8)
		if i < closed && sound > 0.02 {
			c = lighten(pn.col, 0.3)
			pn.glow(p, path[2].X-8, y, 8*sound, alpha(pn.col, 0.4))
		}
		pn.circle(p, path[2].X-8, y, 3.5, c)
	}
	pn.glow(p, bell.X, bell.Y, 30*pn.live.level, alpha(pn.col, 0.5))
	for k, nt := range pn.live.notes {
		if nt.age > 1.2 {
			continue
		}
		ph := float32(nt.age / 1.2)
		g := shaped([]string{"♪", "♫", "♬"}[k%3], 16, true)
		g.Paint(p, geom.Pt(bell.X+4+ph*r.Size().W*0.2+6*sinf(nt.age*6), bell.Y-18-ph*r.Size().H*0.5), alpha(pn.pal.tone(pn.col), 1-ph))
	}
}

// drawFlute draws a flute, its holes covered and open by the note, and
// breath coming out of its end.
func drawFlute(p *paint.Painter, pn pen, r geom.Rect) {
	cy := r.Min.Y + r.Size().H*0.5
	silver := mix(rgb(0xd0d4dc), pn.col, 0.25)
	pn.rect(p, xyxy(r.Min.X+4, cy-7, r.Max.X-4, cy+7), 7, silver)
	pn.rect(p, xyxy(r.Min.X+4, cy-7, r.Min.X+r.Size().W*0.12, cy+7), 7, lighten(silver, 0.2))
	pn.circle(p, r.Min.X+r.Size().W*0.09, cy, 3, rgb(0x303038))
	var key uint8
	sound := float32(0)
	for _, n := range pn.live.notes {
		if h := held(n); h > sound {
			sound, key = h, n.key
		}
	}
	// The holes: covered from the top, fewer as the note climbs.
	open := 0
	if sound > 0.02 {
		open = int(pn.pos(key) * 6.99)
	}
	for i := range 6 {
		x := r.Min.X + r.Size().W*(0.3+0.1*float32(i))
		covered := i < 6-open
		c := rgb(0x303038)
		if covered && sound > 0.02 {
			c = lighten(pn.col, 0.2)
		}
		pn.circle(p, x, cy, 4.5, c)
	}
	// Breath from the far end, a stream of motes.
	if sound > 0.02 {
		for j := range 8 {
			ph := float32(math.Mod(pn.t*1.3+float64(j)/8, 1))
			x := r.Max.X - 4 + ph*0
			x -= 4
			x += ph * 30
			y := cy + 10*sinf(pn.t*4+float64(j))*ph
			pn.circle(p, x, y, 1.5+3*ph, alpha(pn.col, sound*(1-ph)*0.7))
		}
		pn.glow(p, r.Max.X-10, cy, 30*sound, alpha(pn.col, 0.5))
	}
}

// drawScope draws a synth's oscilloscope: the wave of the note it plays,
// square or saw or sine by its program, as high as it is loud and as
// close as its pitch is high.
func drawScope(p *paint.Painter, pn pen, r geom.Rect) {
	pn.rect(p, r, 8, rgb(0x061410))
	grid := alpha(pn.col, 0.12)
	for i := 1; i < 6; i++ {
		x := r.Min.X + r.Size().W*float32(i)/6
		pn.rect(p, xyxy(x, r.Min.Y, x+1, r.Max.Y), 0, grid)
	}
	for i := 1; i < 4; i++ {
		y := r.Min.Y + r.Size().H*float32(i)/4
		pn.rect(p, xyxy(r.Min.X, y, r.Max.X, y+1), 0, grid)
	}
	var key uint8 = 60
	amp := float32(0.05)
	for _, n := range pn.live.notes {
		if h := held(n); h > amp {
			amp, key = h, n.key
		}
	}
	cycles := 1.5 + 3*float64(pn.pos(key))
	cy := r.Min.Y + r.Size().H/2
	a := amp * r.Size().H * 0.38
	wave := func(x float64) float32 {
		ph := math.Mod(x*cycles+pn.t*0.7, 1)
		switch pn.prog {
		case 80:
			if ph < 0.5 {
				return 1
			}
			return -1
		case 81, 84, 87:
			return float32(2*ph - 1)
		case 82, 83:
			return float32(1 - 4*math.Abs(ph-0.5))
		}
		return sinf(ph * 2 * math.Pi)
	}
	const steps = 64
	prev := geom.Pt(r.Min.X, cy-a*wave(0))
	c := lighten(pn.col, 0.3)
	for i := 1; i <= steps; i++ {
		x := float64(i) / steps
		pt := geom.Pt(r.Min.X+float32(x)*r.Size().W, cy-a*wave(x))
		pn.line(p, prev, pt, 2, c)
		prev = pt
	}
	pn.glow(p, r.Min.X+r.Size().W/2, cy, r.Size().W*0.5*amp, alpha(pn.col, 0.35))
}

// drawAurora draws a pad as light: soft clouds of its colour drifting,
// as bright as it sounds.
func drawAurora(p *paint.Painter, pn pen, r geom.Rect) {
	lv := min(1, pn.live.level*1.2) + 0.1
	for i := range 4 {
		ph := pn.t*0.25 + float64(i)*1.7
		cx := r.Min.X + r.Size().W*(0.5+0.35*sinf(ph))
		cy := r.Min.Y + r.Size().H*(0.5+0.25*cosf(ph*1.3))
		c := pn.col
		if i%2 == 1 {
			c = mix(pn.col, pn.pal.accent2, 0.6)
		}
		pn.glow(p, cx, cy, r.Size().H*(0.5+0.2*float32(i%3))*lv, alpha(c, 0.55*lv))
	}
	for _, n := range pn.live.notes {
		x := r.Min.X + r.Size().W*pn.pos(n.key)
		h := held(n)
		pn.rect(p, xyxy(x-1, r.Min.Y+r.Size().H*(1-h*0.8), x+1, r.Max.Y), 1, alpha(lighten(pn.col, 0.5), h*0.6))
	}
}

// drawSparkle draws a synth effect: sparks flying out from the middle
// with each note, and stars twinkling.
func drawSparkle(p *paint.Painter, pn pen, r geom.Rect) {
	cx, cy := r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2
	for i := range 14 {
		x := r.Min.X + r.Size().W*float32(math.Mod(float64(i)*0.618, 1))
		y := r.Min.Y + r.Size().H*float32(math.Mod(float64(i)*0.371+0.2, 1))
		tw := 0.5 + 0.5*sinf(pn.t*3+float64(i))
		pn.glow(p, x, y, 6*tw*(0.3+pn.live.level), alpha(pn.pal.ink, 0.6))
	}
	for k, n := range pn.live.notes {
		if n.age > 1 {
			continue
		}
		x := float32(n.age)
		for j := range 8 {
			a := float64(j)/8*2*math.Pi + float64(k)
			d := x * r.Size().H * 0.7
			pn.glow(p, cx+cosf(a)*d, cy+sinf(a)*d, 10*(1-x)*n.vel, alpha(pn.col, 1-x))
		}
	}
}

// drawKalimba draws a kalimba's tines, each shaking as it is plucked.
func drawKalimba(p *paint.Painter, pn pen, r geom.Rect) {
	n := 9
	var amp [9]float32
	var age [9]float64
	for _, nt := range pn.live.notes {
		i := int(pn.pos(nt.key) * float32(n-1))
		if s := strike(nt, 3); s > amp[i] {
			amp[i], age[i] = s, nt.age
		}
	}
	pn.rect(p, r, 10, mix(rgb(0x6b4a2b), pn.col, 0.15))
	pn.circle(p, r.Min.X+r.Size().W/2, r.Max.Y-r.Size().H*0.28, r.Size().H*0.16, rgb(0x2a1a10))
	w := r.Size().W * 0.8 / float32(n)
	for i := range n {
		// The longest tines in the middle, as a kalimba's.
		d := math.Abs(float64(i) - 4)
		l := r.Size().H * float32(0.75-0.09*d)
		x := r.Min.X + r.Size().W*0.1 + (float32(i)+0.5)*w
		shake := amp[i] * 3 * sinf(age[i]*80)
		pn.rect(p, xyxy(x-w*0.25+shake, r.Min.Y+4, x+w*0.25+shake, r.Min.Y+4+l), 2, mix(rgb(0xc8ccd4), pn.col, amp[i]))
		pn.glow(p, x, r.Min.Y+l*0.6, 18*amp[i], alpha(pn.col, 0.7))
	}
}

// drawBag draws a bagpipe: its bag swelling with the sound, and its
// drones.
func drawBag(p *paint.Painter, pn pen, r geom.Rect) {
	cx, cy := r.Min.X+r.Size().W*0.45, r.Min.Y+r.Size().H*0.6
	sw := 1 + 0.12*pn.live.level
	bw, bh := r.Size().W*0.32*sw, r.Size().H*0.36*sw
	for i := range 3 {
		x := cx - bw*0.3 + float32(i)*bw*0.35
		pn.line(p, geom.Pt(x, cy-bh*0.5), geom.Pt(x+16+float32(i)*6, r.Min.Y+4+float32(i)*6), 4, rgb(0x3a2a20))
		pn.circle(p, x+16+float32(i)*6, r.Min.Y+4+float32(i)*6, 4, lighten(pn.col, 0.3))
	}
	pn.rect(p, xyxy(cx-bw, cy-bh, cx+bw, cy+bh), bh, mix(rgb(0x3b6b3b), pn.col, 0.4))
	for i := range 3 {
		pn.rect(p, xyxy(cx-bw, cy-bh+float32(i)*bh*0.6, cx+bw, cy-bh+float32(i)*bh*0.6+3), 1, alpha(rgb(0xc03030), 0.6))
	}
	pn.line(p, geom.Pt(cx+bw*0.7, cy+bh*0.5), geom.Pt(r.Max.X-6, r.Max.Y-2), 5, rgb(0x2a1a10))
	pn.glow(p, cx, cy, bw*2*pn.live.level, alpha(pn.col, 0.4))
}

// drawPad draws a drum pad, a steel drum or a bell by the program: a
// head that squashes as it is struck, and rings going out.
func drawPad(p *paint.Painter, pn pen, r geom.Rect) {
	cx, cy := r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2
	rad := min(r.Size().W, r.Size().H) * 0.42
	hit := pn.live.hit
	for _, n := range pn.live.notes {
		if n.age < 0.6 {
			x := float32(n.age / 0.6)
			pn.ring(p, cx, cy, rad*(1+x), 2, alpha(pn.col, (1-x)*0.6*n.vel))
		}
	}
	squash := 1 - 0.12*hit
	head := xyxy(cx-rad/squash, cy-rad*squash, cx+rad/squash, cy+rad*squash)
	c := mix(rgb(0xd8d0c0), pn.col, 0.3)
	if pn.prog == 114 {
		c = mix(rgb(0xc0c8d0), pn.col, 0.3)
	}
	pn.rect(p, head, rad, c)
	pn.ring(p, cx, cy, rad*0.98, 4, darken(c, 0.4))
	if pn.prog == 114 {
		// A steel drum's dents, one for each note.
		for i := range 8 {
			a := float64(i)/8*2*math.Pi + 0.2
			lit := float32(0)
			for _, n := range pn.live.notes {
				if int(n.key)%8 == i {
					lit = max(lit, strike(n, 4))
				}
			}
			pn.circle(p, cx+cosf(a)*rad*0.6, cy+sinf(a)*rad*0.6, rad*0.18, mix(darken(c, 0.15), pn.col, lit))
		}
	}
	pn.glow(p, cx, cy, rad*1.5*hit, alpha(pn.col, 0.7))
}

// drawSFX draws a sound effect: the sea's waves, a helicopter's rotor,
// or static, by the program.
func drawSFX(p *paint.Painter, pn pen, r geom.Rect) {
	lv := min(1, pn.live.level+0.05)
	switch pn.prog {
	case 122:
		for k := range 4 {
			const steps = 24
			y0 := r.Min.Y + r.Size().H*(0.3+0.18*float32(k))
			prev := geom.Pt(r.Min.X, y0)
			for i := 1; i <= steps; i++ {
				x := float64(i) / steps
				pt := geom.Pt(r.Min.X+float32(x)*r.Size().W, y0+6*lv*sinf(x*8+pn.t*(1.5+float64(k)*0.3)))
				pn.line(p, prev, pt, 2, alpha(mix(pn.col, pn.pal.accent2, float32(k)/4), 0.4+0.5*lv))
				prev = pt
			}
		}
	case 125:
		cx, cy := r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2
		a := float32(pn.t * 25 * float64(lv))
		end := p.Push(paint.Rotate(a, geom.Pt(cx, cy)))
		pn.rect(p, xyxy(cx-r.Size().W*0.45, cy-3, cx+r.Size().W*0.45, cy+3), 3, alpha(pn.pal.ink, 0.7))
		end()
		pn.circle(p, cx, cy, 8, pn.col)
	default:
		// Static: specks flickering where the sound is.
		seed := int(pn.t * 30)
		for i := range 60 {
			h := uint32((seed*7919 + i*104729) * 2654435761)
			x := r.Min.X + r.Size().W*float32(h%1000)/1000
			y := r.Min.Y + r.Size().H*float32(h/1000%1000)/1000
			if float32(h%97)/97 > lv {
				continue
			}
			pn.rect(p, xyxy(x, y, x+3, y+3), 0, alpha(mix(pn.pal.ink, pn.col, float32(h%3)/2), 0.7))
		}
	}
	pn.glow(p, r.Min.X+r.Size().W/2, r.Min.Y+r.Size().H/2, r.Size().H*lv, alpha(pn.col, 0.3))
}

// The parts of the drum kit.
const (
	kitKick = iota
	kitSnare
	kitHat
	kitTom1
	kitTom2
	kitFloor
	kitCrash
	kitRide
	kitAux
	kitParts
)

// kitPart returns the part of the kit drum key plays.
func kitPart(key uint8) int {
	switch key {
	case 35, 36:
		return kitKick
	case 37, 38, 39, 40:
		return kitSnare
	case 42, 44, 46:
		return kitHat
	case 48, 50:
		return kitTom1
	case 45, 47:
		return kitTom2
	case 41, 43:
		return kitFloor
	case 49, 52, 55, 57:
		return kitCrash
	case 51, 53, 59:
		return kitRide
	}
	return kitAux
}

// drawKit draws a drum kit, each drum bouncing as it is hit, the
// cymbals rocking, the hi-hat opening, and the whole kit jumping with
// the kick.
func drawKit(p *paint.Painter, pn pen, r geom.Rect) {
	var hit [kitParts]float32
	var age [kitParts]float64
	open := float32(0)
	for _, n := range pn.live.notes {
		k := kitPart(n.key)
		if s := strike(liveNote{vel: n.vel, age: n.age, left: 1}, 7); s > hit[k] {
			hit[k], age[k] = s, n.age
		}
		if n.key == 46 && n.age < 0.4 {
			open = 1 - float32(n.age/0.4)
		}
	}
	w, h := r.Size().W, r.Size().H
	jump := hit[kitKick] * 3
	ox, oy := r.Min.X, r.Min.Y-jump
	at := func(fx, fy float32) (float32, float32) { return ox + fx*w, oy + fy*h }
	shell := func(fx, fy, fw, fh float32, k int, c color.NRGBA) {
		x, y := at(fx, fy)
		dw, dh := fw*w, fh*h
		sq := 1 - 0.15*hit[k]
		pn.rect(p, xyxy(x-dw/2, y-dh/2*sq, x+dw/2, y+dh/2*sq), dh/2, mix(c, pn.col, 0.5*hit[k]))
		pn.rect(p, xyxy(x-dw/2, y-dh/2*sq, x+dw/2, y-dh/2*sq+dh*0.35), dh/2, mix(rgb(0xece6d8), pn.col, hit[k]*0.7))
		pn.glow(p, x, y, dw*hit[k], alpha(pn.col, 0.7))
	}
	cymbal := func(fx, fy, fw float32, k int) {
		x, y := at(fx, fy)
		dw := fw * w
		rock := hit[k] * 0.3 * sinf(age[k]*22)
		end := p.Push(paint.Rotate(rock, geom.Pt(x, y)))
		pn.rect(p, xyxy(x-dw/2, y-3, x+dw/2, y+3), 3, mix(rgb(0xd9b44a), pn.col, hit[k]*0.6))
		pn.circle(p, x, y-2, 3, rgb(0x8a6a20))
		end()
		pn.glow(p, x, y, dw*0.9*hit[k], alpha(rgb(0xffe08a), 0.7))
		x2, y2 := at(fx, 0.98)
		pn.line(p, geom.Pt(x, y+3), geom.Pt(x2, y2), 2, alpha(pn.pal.ink, 0.25))
	}
	// Stands and cymbals behind, then the toms, then the kick in front.
	cymbal(0.14, 0.18, 0.28, kitCrash)
	cymbal(0.86, 0.22, 0.3, kitRide)
	// The hi-hat: two cymbals that part as it opens.
	hx, hy := at(0.1, 0.48)
	gap := 3 + 6*open
	hw := 0.2 * w
	pn.rect(p, xyxy(hx-hw/2, hy-gap-3+hit[kitHat]*2, hx+hw/2, hy-gap+hit[kitHat]*2), 2, mix(rgb(0xd9b44a), pn.col, hit[kitHat]))
	pn.rect(p, xyxy(hx-hw/2, hy, hx+hw/2, hy+3), 2, rgb(0xb8962a))
	pn.glow(p, hx, hy, hw*hit[kitHat], alpha(rgb(0xffe08a), 0.6))
	shell(0.38, 0.32, 0.16, 0.2, kitTom1, rgb(0x2a3a8a))
	shell(0.6, 0.32, 0.17, 0.22, kitTom2, rgb(0x2a3a8a))
	shell(0.82, 0.62, 0.2, 0.28, kitFloor, rgb(0x2a3a8a))
	shell(0.24, 0.66, 0.2, 0.2, kitSnare, rgb(0xb8bcc8))
	// The kick: a big round head facing out, its logo the channel's
	// colour.
	kx, ky := at(0.5, 0.72)
	kr := min(0.24*w, 0.3*h) * (1 + 0.06*hit[kitKick])
	pn.circle(p, kx, ky, kr, rgb(0x1a2470))
	pn.circle(p, kx, ky, kr*0.86, mix(rgb(0xece6d8), pn.col, hit[kitKick]*0.5))
	pn.circle(p, kx, ky, kr*0.32, pn.col)
	pn.glow(p, kx, ky, kr*2*hit[kitKick], alpha(pn.col, 0.7))
	// The rest of the percussion: little lights along the bottom.
	if hit[kitAux] > 0.01 {
		x, y := at(0.5, 0.06)
		for i := range 5 {
			pn.glow(p, x+float32(i-2)*14, y, 10*hit[kitAux], alpha(pn.pal.accent2, 0.8))
		}
	}
}
