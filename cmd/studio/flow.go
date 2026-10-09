package main

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// flow is the music as it flows past: the notes of each track, low to
// high, sliding left past the line where they are heard, those of the
// bar to come already in sight; the drums in lanes under them; and the
// chords above. Each track's notes are joined, so an arpeggio shows its
// shape as it runs.
type flow struct {
	s    Studio
	size geom.Size
	// lo and hi are the pitches shown, easing to the notes in sight.
	lo, hi float32
}

// The bars in sight: so many before the line where notes are heard, and
// so many after.
const (
	pastBars   = 1.6
	futureBars = 1.1
)

func newFlow() *flow { return &flow{lo: 48, hi: 84} }

func (fl *flow) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	fl.size = c.Max
	return c.Max
}

func (fl *flow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	s := &fl.s
	v := geom.Rc(0, 0, box.W, box.H)
	p.RRect(v, 14, paint.Solid(color.NRGBA{0x12, 0x14, 0x1d, 0xff}))
	if s.BarFrames <= 0 {
		return
	}
	end := p.Layer(paint.LayerOpts{Bounds: v, Opacity: 1, Clip: true, Radius: 14})
	defer end()
	ink := widget.Ink.Get(f.Theme)
	heard := heardAt(s.Clock, f.Now)
	bar := s.BarFrames
	from := heard - pastBars*bar
	span := (pastBars + futureBars) * bar
	x := func(frame float64) float32 { return v.Min.X + float32((frame-from)/span)*v.Size().W }
	now := x(heard)

	// The lanes: the chords on top, the drums at the bottom, a lane a
	// drum track, and the notes between.
	const chordH = 24
	var drums []int
	lane := map[int]int{}
	for i, t := range s.Tracks {
		if t.Drums {
			lane[i] = len(drums)
			drums = append(drums, i)
		}
	}
	laneH := float32(14)
	drumTop := v.Max.Y - laneH*float32(len(drums)) - 6
	notes := geom.Rc(v.Min.X, v.Min.Y+chordH+8, v.Size().W, drumTop-v.Min.Y-chordH-14)

	// Beats and bars.
	beats := max(s.Beats, 1)
	beat := bar / float64(beats)
	first := math.Floor((from-float64(s.BarFrame))/beat) * beat
	for at := float64(s.BarFrame) + first; at < from+span; at += beat {
		k := int(math.Round((at - float64(s.BarFrame)) / beat))
		a := float32(0.05)
		if k%beats == 0 {
			a = 0.13
			b := s.Bar + k/beats
			if s.PhraseBars > 0 && ((b%s.PhraseBars)+s.PhraseBars)%s.PhraseBars == 0 {
				a = 0.28
			}
		}
		xx := x(at)
		p.RRect(geom.Rc(xx-0.5, v.Min.Y+chordH+4, 1, v.Size().H-chordH-4), 0, paint.Solid(withAlpha(ink, a)))
	}

	// The chords.
	for _, c := range s.Spans {
		x0, x1 := x(float64(c.Frame)), x(float64(c.Frame+c.Len))
		col := chordColor(c.Index)
		heardNow := float64(c.Frame) <= heard && heard < float64(c.Frame+c.Len)
		fill := withAlpha(grey(col, 0.55), 0.3)
		if heardNow {
			fill = withAlpha(col, 0.85)
		}
		r := geom.Rc(x0+2, v.Min.Y+5, x1-x0-4, chordH-4)
		p.RRect(r, 7, paint.Solid(fill))
		run := audioui.Shaped(c.Name, 12.5, heardNow, false)
		tc := withAlpha(ink, 0.7)
		if heardNow {
			tc = color.NRGBA{0x10, 0x12, 0x1a, 0xff}
		}
		if run.Advance < r.Size().W-8 {
			run.Paint(p, geom.Pt(r.Min.X+8, r.Min.Y+(r.Size().H-run.Height())/2), tc)
		}
	}

	// The notes, high to low, and the lines that join each track's.
	rows := max(fl.hi-fl.lo, 12)
	rowH := min(max(notes.Size().H/rows, 3), 11)
	y := func(pitch int) float32 {
		return notes.Min.Y + (fl.hi-float32(pitch))/rows*notes.Size().H - rowH/2
	}
	type last struct {
		x, y float32
		at   float64
		ok   bool
	}
	prev := make([]last, len(s.Tracks)+1)
	for _, n := range s.Notes {
		if n.Drum {
			continue
		}
		c := keypadColor
		ti := len(s.Tracks)
		if n.Track >= 0 && n.Track < len(s.Tracks) {
			c, ti = hexColor(s.Tracks[n.Track].Color, n.Track), n.Track
		}
		x0, x1 := x(float64(n.Frame)), x(float64(n.Frame+n.Len))
		if x1 < v.Min.X || x0 > v.Max.X {
			continue
		}
		yy := y(n.Pitch)
		if pv := prev[ti]; pv.ok && float64(n.Frame)-pv.at < 1.01*beat && pv.y != yy {
			audioui.Segment(p, geom.Pt(pv.x, pv.y+rowH/2), geom.Pt(x0, yy+rowH/2), 1.2, withAlpha(c, 0.35))
		}
		prev[ti] = last{x: x1, y: yy, at: float64(n.Frame), ok: true}
		r := geom.Rc(x0, yy, max(x1-x0-1, 3), rowH-1)
		switch {
		case heard < float64(n.Frame):
			// To come: an outline of the note.
			p.RRectStroke(r, rowH/2, paint.Solid(withAlpha(c, 0.08)), paint.Stroke{Width: 1, Color: withAlpha(c, 0.45)})
		case heard < float64(n.Frame+n.Len):
			// Heard now: bright, and glowing.
			p.ShadowRRect(r, rowH/2, paint.Solid(lighten(c, 0.35)), paint.Shadow{Blur: 10, Color: withAlpha(c, 0.9)})
		default:
			age := float32((heard - float64(n.Frame+n.Len)) / bar)
			p.RRect(r, rowH/2, paint.Solid(withAlpha(c, 0.75-0.4*min(age, 1))))
		}
	}

	// The drums.
	for _, n := range s.Notes {
		if !n.Drum || n.Track < 0 {
			continue
		}
		l, ok := lane[n.Track]
		if !ok {
			continue
		}
		xx := x(float64(n.Frame))
		if xx < v.Min.X-8 || xx > v.Max.X+8 {
			continue
		}
		c := hexColor(s.Tracks[n.Track].Color, n.Track)
		d := 4 + 5*n.Vel
		cy := drumTop + laneH*float32(l) + laneH/2
		r := geom.Rc(xx-d/2, cy-d/2, d, d)
		age := heard - float64(n.Frame)
		switch {
		case age < 0:
			p.RRectStroke(r, d/2, paint.Solid(withAlpha(c, 0.05)), paint.Stroke{Width: 1, Color: withAlpha(c, 0.4)})
		case age < 0.12*audioRate:
			g := 1 - float32(age/(0.12*audioRate))
			big := geom.Rc(xx-d*(0.5+0.5*g), cy-d*(0.5+0.5*g), d*(1+g), d*(1+g))
			p.ShadowRRect(big, big.Size().W/2, paint.Solid(lighten(c, 0.4*g)), paint.Shadow{Blur: 8, Color: withAlpha(c, g)})
		default:
			p.RRect(r, d/2, paint.Solid(withAlpha(c, 0.6)))
		}
	}
	for i, t := range drums {
		run := audioui.Shaped(s.Tracks[t].Name, 10, false, false)
		at := geom.Pt(v.Min.X+8, drumTop+laneH*float32(i)+(laneH-run.Height())/2)
		p.RRect(geom.Rc(at.X-4, at.Y-1, run.Advance+8, run.Height()+2), 4, paint.Solid(color.NRGBA{0x12, 0x14, 0x1d, 0xd8}))
		run.Paint(p, at, withAlpha(ink, 0.55))
	}

	// The line where notes are heard.
	p.ShadowRRect(geom.Rc(now-1, v.Min.Y+chordH+4, 2, v.Size().H-chordH-4), 1, paint.Solid(withAlpha(ink, 0.85)),
		paint.Shadow{Blur: 8, Color: withAlpha(audioui.Sound.Get(f.Theme), 0.8)})
}

// Step eases the pitches shown toward those of the notes in sight.
func (fl *flow) Step(dt time.Duration) bool {
	lo, hi := float32(200), float32(-1)
	for _, n := range fl.s.Notes {
		if !n.Drum && n.Pitch > 0 {
			lo, hi = min(lo, float32(n.Pitch)), max(hi, float32(n.Pitch))
		}
	}
	if hi < 0 {
		lo, hi = 48, 84
	}
	lo, hi = lo-3, hi+3
	if hi-lo < 24 {
		mid := (lo + hi) / 2
		lo, hi = mid-12, mid+12
	}
	k := min(1, float32(dt.Seconds())*3)
	fl.lo += (lo - fl.lo) * k
	fl.hi += (hi - fl.hi) * k
	return true
}
