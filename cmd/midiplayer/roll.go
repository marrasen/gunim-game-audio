package main

import (
	"fmt"
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// roll is the song falling onto a piano: each note a bar in its
// channel's colour that falls toward its key, as long as it lasts, and
// lights the key as it lands. The bars and beats scroll down behind
// them. The piano spans the song's notes, whole octaves of them.
type roll struct {
	l      *look
	bounds geom.Rect
	gen    int
	lo, hi int
}

// ahead is how many seconds of the song the roll shows coming.
const ahead = 2.6

func (r *roll) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	if sc := r.l.score; sc != nil && r.gen != r.l.s.ScoreGen {
		r.gen = r.l.s.ScoreGen
		r.lo, r.hi = 127, 0
		for i, ch := range sc.Channels {
			if ch.Notes == 0 || ch.Drums || i == 9 {
				continue
			}
			r.lo, r.hi = min(r.lo, int(ch.Low)), max(r.hi, int(ch.High))
		}
		if r.lo > r.hi {
			r.lo, r.hi = 48, 84
		}
		r.lo -= r.lo % 12
		r.hi += 11 - r.hi%12
		for r.hi-r.lo < 48 {
			r.lo, r.hi = max(r.lo-12, 0), min(r.hi+12, 127)
			if r.lo == 0 && r.hi == 127 {
				break
			}
		}
	}
	if r.hi == 0 {
		r.lo, r.hi = 36, 95
	}
	return c.Max
}

func (r *roll) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := r.l
	pal := l.pal
	pn := pen{pal: pal}
	full := geom.Rect{Max: box.Point()}
	rad := 18 * pal.round
	p.ShadowRRect(full, rad, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, 0), To: geom.Pt(0, box.H),
		Start: alpha(darken(pal.panel, 0.25), 0.92), End: alpha(pal.panel, 0.95)}},
		paint.Shadow{Blur: 20, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.35)})
	p.RRectStroke(full, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.7)})
	end := p.Layer(paint.LayerOpts{Bounds: full, Opacity: 1, Clip: true, Radius: rad})
	defer end()
	keyH := min(max(box.H*0.28, 54), 96)
	keyTop := box.H - keyH
	xs := keyLayout(r.lo, r.hi, 0, box.W)
	for k := range xs {
		l.keyX[k] = [2]float32{xs[k][0] + r.bounds.Min.X, xs[k][1] + r.bounds.Min.X}
	}
	l.keyY = keyTop + r.bounds.Min.Y
	fallH := keyTop
	yOf := func(t float64) float32 { return keyTop - float32((t-l.t)/ahead)*fallH }
	// The lanes of the black keys, faintly, for the eye to follow.
	for k := r.lo; k <= r.hi; k++ {
		if black(k) {
			p.RRect(xyxy(xs[k][0], 0, xs[k][1], keyTop), 0, paint.Solid(alpha(pal.bgBottom, 0.25)))
		}
		if k%12 == 0 {
			p.RRect(xyxy(xs[k][0], 0, xs[k][0]+1, keyTop), 0, paint.Solid(alpha(pal.edge, 0.35)))
		}
	}
	sc := l.score
	if sc == nil {
		r.paintKeys(p, pn, xs, keyTop, box, nil)
		return
	}
	// The beats, and the bars numbered.
	b0, _ := sc.beatAt(l.t - 0.2)
	for b := max(b0, 0); b < len(sc.Beats); b++ {
		bt := sc.Beats[b]
		if bt > l.t+ahead {
			break
		}
		y := yOf(bt)
		bar := sc.BarBeats > 0 && b%sc.BarBeats == 0
		c := alpha(pal.edge, 0.25)
		if bar {
			c = alpha(pal.edge, 0.6)
			n := shaped(fmt.Sprint(b/max(sc.BarBeats, 1)+1), 10, false)
			n.Paint(p, geom.Pt(6, y-n.Height()-2), alpha(pal.dim, 0.7))
		}
		p.RRect(xyxy(0, y, box.W, y+1), 0, paint.Solid(c))
	}
	// The notes, falling. A note sounding glows, and a note to come is
	// dimmer the further off it is.
	var lit [128]color.NRGBA
	var litV [128]float32
	sc.span(l.t-0.05, l.t+ahead, func(n *Note) {
		if sc.Channels[n.Ch].Drums || int(n.Key) < r.lo || int(n.Key) > r.hi {
			return
		}
		muted := l.s.Muted[n.Ch] || l.s.Solo >= 0 && l.s.Solo != int(n.Ch)
		x0, x1 := xs[n.Key][0], xs[n.Key][1]
		y0, y1 := yOf(n.End), yOf(n.Start)
		y1 = min(y1, keyTop)
		if y1-y0 < 3 {
			y0 = y1 - 3
		}
		col := pal.ch[n.Ch]
		sounding := n.Start <= l.t && n.End > l.t
		a := float32(0.85)
		if muted {
			a = 0.18
		}
		c := alpha(col, a)
		if sounding && !muted {
			c = lighten(col, 0.35)
			v := float32(n.Vel) / 127
			if v >= litV[n.Key] {
				lit[n.Key], litV[n.Key] = col, v
			}
		}
		rc := xyxy(x0+1, y0, x1-1, y1)
		pn.rect(p, rc, min(4, (x1-x0)/2), c)
		if !muted && pal.pixel < 1.5 {
			p.RRect(xyxy(x0+2, y0+1, x1-2, min(y0+4, y1)), 2, paint.Solid(alpha(pal.white, 0.25)))
		}
	})
	// The splash where notes land on the keys.
	func() {
		defer p.Blend(paint.BlendAdd)()
		for k := r.lo; k <= r.hi; k++ {
			if litV[k] == 0 {
				continue
			}
			cx := (xs[k][0] + xs[k][1]) / 2
			pn.glow(p, cx, keyTop, 26+20*litV[k], alpha(lit[k], 0.9))
			p.RRect(xyxy(xs[k][0], keyTop-60*litV[k], xs[k][1], keyTop), 0,
				paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, keyTop-60*litV[k]), To: geom.Pt(0, keyTop),
					Start: alpha(lit[k], 0), End: alpha(lit[k], 0.5)}})
		}
	}()
	r.paintKeys(p, pn, xs, keyTop, box, &lit)
}

// paintKeys draws the piano, its keys lit and down where notes sound.
func (r *roll) paintKeys(p *paint.Painter, pn pen, xs [128][2]float32, top float32, box geom.Size, lit *[128]color.NRGBA) {
	pal := pn.pal
	p.RRect(xyxy(0, top-3, box.W, top), 0, paint.Solid(alpha(pal.accent, 0.6)))
	on := func(k int) (color.NRGBA, bool) {
		if lit == nil || lit[k].A == 0 {
			return color.NRGBA{}, false
		}
		return lit[k], true
	}
	for k := r.lo; k <= r.hi; k++ {
		if black(k) {
			continue
		}
		c := rgb(0xf4f2ea)
		down := float32(0)
		if lc, ok := on(k); ok {
			c, down = mix(c, lc, 0.75), 2
		}
		pn.rect(p, xyxy(xs[k][0]+0.5, top+down, xs[k][1]-0.5, box.H+4), 3, c)
		if k%12 == 0 && xs[k][1]-xs[k][0] > 9 {
			n := shaped(fmt.Sprintf("C%d", k/12-1), 9, false)
			n.Paint(p, geom.Pt((xs[k][0]+xs[k][1])/2-n.Advance/2, box.H-n.Height()-4), rgb(0x8a8680))
		}
	}
	for k := r.lo; k <= r.hi; k++ {
		if !black(k) {
			continue
		}
		c := rgb(0x16161c)
		down := float32(0)
		if lc, ok := on(k); ok {
			c, down = mix(c, lc, 0.85), 2
		}
		pn.rect(p, xyxy(xs[k][0], top+down, xs[k][1], top+(box.H-top)*0.62+down), 2, c)
	}
}
