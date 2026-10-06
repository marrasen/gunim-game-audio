package main

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// stage is the song in 3D: each track an orb on a ring, the rings its
// tiers, tier 1 innermost; each note a spark that flies from its orb to
// the core in the middle, high or low by its pitch, so an arpeggio
// climbs and falls; each drum a ripple on the floor; and the chords of
// the progression round the core, the one heard raised and lit. A drag
// turns the stage, and a tap on an orb starts or stops its track.
type stage struct {
	s                         Studio
	orb, disc, core, box, rod *paint.Mesh
	size                      geom.Size
	// turn is the stage's angle, and spin how fast it turns, easing back
	// to its own pace after a drag.
	turn, spin float32
	dragging   bool
	dragAt     geom.Point
	downAt     geom.Point
	// levels are the tracks' levels as the orbs show them, easing.
	levels []float32
	// chordAt counts down from 1 as a new chord comes in.
	chordAt  float32
	chordWas int
	// items says which track each item of the scene last drawn shows, -1
	// for none, for a tap to find.
	items []int
	last  paint.Scene
}

const idleSpin = 0.07

func newStage() *stage {
	white := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	return &stage{
		orb:  paint.NewSphere(16, 28, white),
		disc: paint.NewSphere(4, 40, white),
		core: paint.NewSphere(24, 40, white),
		box:  paint.NewBox(geom.V3(1, 1, 1), white),
		rod:  paint.NewBox(geom.V3(1, 1, 1), white),
		spin: idleSpin, chordWas: -1,
	}
}

func (st *stage) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	st.size = c.Max
	return c.Max
}

// heardAt returns the frame heard at t, from the clock.
func heardAt(c Clock, t time.Time) float64 {
	return float64(c.Frame) + t.Sub(c.At).Seconds()*c.Rate
}

// hexColor reads a colour as #rrggbb, or returns the palette's i'th.
func hexColor(s string, i int) color.NRGBA {
	if len(s) == 7 && s[0] == '#' {
		if v, err := strconv.ParseUint(s[1:], 16, 32); err == nil {
			return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
		}
	}
	return palette[i%len(palette)]
}

var palette = []color.NRGBA{
	{0x7c, 0x8c, 0xff, 0xff}, {0x3f, 0xd0, 0xc9, 0xff}, {0xff, 0xcf, 0x5c, 0xff}, {0x5c, 0xe1, 0xa8, 0xff},
	{0xff, 0x6b, 0x9d, 0xff}, {0xff, 0x8a, 0x4c, 0xff}, {0xe9, 0xf2, 0x7a, 0xff}, {0xc5, 0x8c, 0xff, 0xff},
}

// keypadColor is the colour of the keypad's notes.
var keypadColor = color.NRGBA{0xff, 0xff, 0xff, 0xff}

// withAlpha returns c at alpha a, from 0 to 1.
func withAlpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(min(max(a, 0), 1) * 255)
	return c
}

// lighten blends c toward white by t.
func lighten(c color.NRGBA, t float32) color.NRGBA {
	m := func(x uint8) uint8 { return uint8(float32(x) + (255-float32(x))*t) }
	return color.NRGBA{m(c.R), m(c.G), m(c.B), c.A}
}

// grey takes c toward a grey of its brightness by t.
func grey(c color.NRGBA, t float32) color.NRGBA {
	y := 0.3*float32(c.R) + 0.59*float32(c.G) + 0.11*float32(c.B)
	m := func(x uint8) uint8 { return uint8(float32(x) + (y*0.7-float32(x))*t) }
	return color.NRGBA{m(c.R), m(c.G), m(c.B), c.A}
}

// orbAt returns where track i's orb stands: on the ring of its tier,
// tier 1 innermost, or in a wandering song the core tracks inside and
// the rest out.
func (st *stage) orbAt(i int) geom.Vec3 {
	tracks := st.s.Tracks
	ring := func(t TrackRow) int {
		if st.s.Tiers == 0 {
			if t.Core {
				return 1
			}
			return 2
		}
		return max(t.Tier, 1)
	}
	r := ring(tracks[i])
	n, k := 0, 0
	for j, t := range tracks {
		if ring(t) == r {
			if j < i {
				k++
			}
			n++
		}
	}
	radius := 1.7 + float32(r-1)*0.85
	if st.s.Tiers == 0 {
		radius = 1.6 + float32(r-1)*1.5
	}
	a := float64(k)/float64(max(n, 1))*2*math.Pi + float64(r)*0.7
	return geom.V3(radius*float32(math.Sin(a)), 0.45, radius*float32(math.Cos(a)))
}

// chordColor returns the colour of chord i of the progression.
func chordColor(i int) color.NRGBA {
	hues := []color.NRGBA{{0xff, 0x9e, 0x5e, 0xff}, {0x6c, 0xc8, 0xff, 0xff}, {0xb4, 0x8c, 0xff, 0xff}, {0x6e, 0xe7, 0xa0, 0xff},
		{0xff, 0x7a, 0xa8, 0xff}, {0xff, 0xd8, 0x6a, 0xff}, {0x7d, 0xe3, 0xe0, 0xff}, {0xd9, 0x9c, 0xff, 0xff}}
	return hues[((i%len(hues))+len(hues))%len(hues)]
}

// camera returns the camera, turned by the stage's angle.
func (st *stage) camera() paint.Camera {
	r := float32(7.8)
	return paint.Camera{Eye: geom.V3(r*float32(math.Sin(float64(st.turn))), 4.2, r*float32(math.Cos(float64(st.turn)))),
		At: geom.V3(0, 0.55, 0), FOV: 0.72}
}

// scene returns the stage as it stands at frame heard.
func (st *stage) scene(heard float64) paint.Scene {
	s := &st.s
	items := st.items[:0]
	sc := paint.Scene{Camera: st.camera(), Light: paint.Light{Direction: geom.V3(-0.4, -1, -0.5),
		Color: color.NRGBA{0xd8, 0xd8, 0xe0, 0xff}, Ambient: color.NRGBA{0x50, 0x50, 0x60, 0xff}}}
	add := func(track int, it paint.SceneItem) {
		sc.Items = append(sc.Items, it)
		items = append(items, track)
	}
	beat := 0.0
	beats := float64(max(s.Beats, 1))
	if s.BarFrames > 0 {
		beat = (heard - float64(s.BarFrame)) / s.BarFrames * beats
	}
	pulse := float32(math.Exp(-6 * (beat - math.Floor(beat))))
	if s.Clock.Rate == 0 {
		pulse = 0
	}
	// The floor, a dark disc, and the rings of the tiers on it.
	add(-1, paint.SceneItem{Mesh: st.disc, Model: geom.Move3(geom.V3(0, -0.06, 0)).Mul(geom.Scale3(geom.V3(6.2, 0.05, 6.2))),
		Tint: color.NRGBA{0x1b, 0x1e, 0x2c, 0xff}, Shine: 8})
	rings := s.Tiers
	if rings == 0 {
		rings = 2
	}
	for r := 1; r <= rings; r++ {
		radius := 1.7 + float32(r-1)*0.85
		if s.Tiers == 0 {
			radius = 1.6 + float32(r-1)*1.5
		}
		lit := s.Tiers > 0 && r <= s.Tier
		c := color.NRGBA{0x2c, 0x31, 0x46, 0xff}
		if lit {
			c = color.NRGBA{0x3c, 0x45, 0x70, 0xff}
		}
		add(-1, paint.SceneItem{Mesh: st.disc, Model: geom.Move3(geom.V3(0, -0.035+float32(r)*0.002, 0)).Mul(geom.Scale3(geom.V3(radius+0.08, 0.02, radius+0.08))),
			Tint: c})
		add(-1, paint.SceneItem{Mesh: st.disc, Model: geom.Move3(geom.V3(0, -0.03+float32(r)*0.002, 0)).Mul(geom.Scale3(geom.V3(radius-0.08, 0.02, radius-0.08))),
			Tint: color.NRGBA{0x1b, 0x1e, 0x2c, 0xff}})
	}
	// The core, in the colour of the chord heard, swelling on the beat.
	cc := chordColor(s.Chord)
	coreSize := 0.34 + 0.05*pulse + 0.1*st.chordAt
	add(-1, paint.SceneItem{Mesh: st.core, Model: geom.Move3(geom.V3(0, coreY, 0)).Mul(geom.Scale3(geom.V3(coreSize, coreSize, coreSize))),
		Tint: withAlpha(lighten(cc, 0.15+0.3*st.chordAt), 0.82), Shine: 90})
	add(-1, paint.SceneItem{Mesh: st.core, Model: geom.Move3(geom.V3(0, coreY, 0)).Mul(geom.Scale3(geom.V3(0.16, 0.16, 0.16))),
		Tint: lighten(cc, 0.6), Shine: 30})
	// The progression round the core.
	for i := range s.Chords {
		a := float64(i)/float64(len(s.Chords))*2*math.Pi + float64(st.turn)*0
		x, z := 0.95*float32(math.Sin(a)), 0.95*float32(math.Cos(a))
		h := float32(0.08)
		c := grey(chordColor(i), 0.6)
		if i == s.Chord {
			h = 0.22 + 0.1*st.chordAt
			c = lighten(chordColor(i), 0.2)
		}
		add(-1, paint.SceneItem{Mesh: st.box, Model: geom.Move3(geom.V3(x, h/2, z)).Mul(geom.TurnY(float32(a))).Mul(geom.Scale3(geom.V3(0.16, h, 0.16))),
			Tint: c, Shine: 40})
	}
	// The orbs.
	for i, t := range s.Tracks {
		p := st.orbAt(i)
		c := hexColor(t.Color, i)
		lvl := float32(0)
		if i < len(st.levels) {
			lvl = st.levels[i]
		}
		size := 0.16 + 0.42*float32(math.Sqrt(float64(lvl)))
		var tint color.NRGBA
		if !t.Playing {
			size = 0.15
			tint = withAlpha(grey(c, 0.7), 0.5)
		} else {
			tint = lighten(c, 0.35*lvl)
			p.Y += 0.04 * pulse
		}
		add(i, paint.SceneItem{Mesh: st.orb, Model: geom.Move3(p).Mul(geom.Scale3(geom.V3(size, size, size))), Tint: tint, Shine: 64})
		// A rod from the floor, as tall as the track is loud.
		if t.Playing {
			h := 0.05 + p.Y - size
			add(i, paint.SceneItem{Mesh: st.rod, Model: geom.Move3(geom.V3(p.X, h/2, p.Z)).Mul(geom.Scale3(geom.V3(0.04, h, 0.04))),
				Tint: withAlpha(c, 0.6)})
		}
	}
	// The notes: a spark flying in to the core for a note, a ripple for
	// a drum.
	beatFrames := s.BarFrames / beats
	for _, n := range s.Notes {
		age := heard - float64(n.Frame)
		if age < 0 {
			continue
		}
		c := keypadColor
		from := geom.V3(0, 2.2, 3.6)
		if n.Track >= 0 && n.Track < len(s.Tracks) {
			c = hexColor(s.Tracks[n.Track].Color, n.Track)
			from = st.orbAt(n.Track)
		}
		if n.Drum {
			life := 0.5 * audioRate
			if age > life {
				continue
			}
			x := float32(age / life)
			r := 0.25 + 1.1*x*n.Vel
			add(-1, paint.SceneItem{Mesh: st.disc, Model: geom.Move3(geom.V3(from.X, 0.01, from.Z)).Mul(geom.Scale3(geom.V3(r, 0.01, r))),
				Tint: withAlpha(lighten(c, 0.3), 0.55*(1-x)*(1-x))})
			continue
		}
		life := 2 * beatFrames
		if age > life {
			continue
		}
		x := float32(age / life)
		e := 1 - (1-x)*(1-x)
		to := geom.V3(0, coreY+min(max(float32(n.Pitch-62)*0.04, -0.8), 0.8), 0)
		pos := from.Add(to.Sub(from).Mul(e))
		// An arc up and over on the way.
		pos.Y += 0.7 * float32(math.Sin(float64(x)*math.Pi)) * (0.4 + 0.6*float32(n.Pitch%12)/12)
		size := (0.05 + 0.07*n.Vel) * (1 - 0.6*x)
		sounding := age < float64(n.Len)
		a := 0.9 * (1 - x)
		tint := withAlpha(c, a)
		if sounding {
			tint = withAlpha(lighten(c, 0.4), a)
			size *= 1.3
		}
		add(-1, paint.SceneItem{Mesh: st.orb, Model: geom.Move3(pos).Mul(geom.Scale3(geom.V3(size, size, size))), Tint: tint, Shine: 20})
	}
	st.items = items
	return sc
}

// coreY is how high the core floats over the middle of the stage.
const coreY = 1.45

// audioRate is frames a second, as a float.
const audioRate = 48000.0

// view returns where the scene draws.
func (st *stage) view() geom.Rect { return geom.Rc(0, 0, st.size.W, st.size.H) }

func (st *stage) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	v := st.view()
	p.RRect(v, 18, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(v.Size().W*0.5, v.Size().H*0.35), To: geom.Pt(v.Max.X, v.Max.Y),
		Radial: true, Start: color.NRGBA{0x26, 0x2b, 0x45, 0xff}, End: color.NRGBA{0x0d, 0x0f, 0x18, 0xff}}})
	end := p.Layer(paint.LayerOpts{Bounds: v, Opacity: 1, Clip: true, Radius: 18})
	heard := heardAt(st.s.Clock, f.Now)
	sc := st.scene(heard)
	st.last = sc
	p.Scene(v, sc)
	st.paintLabels(p, f, sc)
	end()
}

// paintLabels names the orbs under them, and the chord heard and the
// next above the stage.
func (st *stage) paintLabels(p *paint.Painter, f gunim.Frame, sc paint.Scene) {
	v := st.view()
	if v.Empty() {
		return
	}
	m := sc.Camera.Matrix(v.Size().W / v.Size().H)
	ink := audioui.Ink.Get(f.Theme)
	for i, t := range st.s.Tracks {
		q := m.Apply(st.orbAt(i).Sub(geom.V3(0, 0.42, 0)))
		if q.Z > 1 || q.Z < -1 {
			continue
		}
		x := v.Min.X + (q.X+1)/2*v.Size().W
		y := v.Min.Y + (1-q.Y)/2*v.Size().H
		run := audioui.Shaped(t.Name, 12, false, false)
		c := withAlpha(ink, 0.55)
		if t.Playing {
			c = withAlpha(ink, 0.9)
		}
		run.Paint(p, geom.Pt(x-run.Advance/2, y+4), c)
	}
	if len(st.s.Chords) > 0 {
		cur := st.s.Chords[st.s.Chord%len(st.s.Chords)]
		next := st.s.Chords[(st.s.Chord+1)%len(st.s.Chords)]
		big := audioui.Shaped(cur, 30, true, false)
		big.Paint(p, geom.Pt(v.Min.X+22, v.Min.Y+18), lighten(chordColor(st.s.Chord), 0.3))
		small := audioui.Shaped("then "+next, 13, false, false)
		small.Paint(p, geom.Pt(v.Min.X+24, v.Min.Y+18+big.Height()+2), withAlpha(ink, 0.6))
	}
	hint := "Drag to turn · tap an orb to start or stop its track"
	run := audioui.Shaped(hint, 11, false, false)
	run.Paint(p, geom.Pt(v.Max.X-run.Advance-16, v.Max.Y-run.Height()-12), withAlpha(ink, 0.4))
}

func (st *stage) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if st.view().Contains(e.Pos) {
			st.dragging, st.dragAt, st.downAt = true, e.Pos, e.Pos
			return true
		}
	case input.PointerMove:
		if st.dragging {
			dx := e.Pos.X - st.dragAt.X
			st.turn -= dx / 160
			st.spin = -dx / 40
			st.dragAt = e.Pos
			u.Invalidate()
			return true
		}
	case input.PointerUp:
		if d := e.Pos.Sub(st.downAt); st.dragging && d.X*d.X+d.Y*d.Y < 6*6 {
			if hit, ok := st.last.Pick(st.view(), e.Pos); ok && hit.Item < len(st.items) {
				if i := st.items[hit.Item]; i >= 0 && i < len(st.s.Tracks) {
					u.Send(st, PartClicked{Name: st.s.Tracks[i].Name})
				}
			}
		}
		st.dragging = false
	}
	return false
}

// Step turns the stage and eases the orbs and the core.
func (st *stage) Step(dt time.Duration) bool {
	t := float32(dt.Seconds())
	if !st.dragging {
		st.spin += (idleSpin - st.spin) * min(1, t*1.5)
		st.turn += st.spin * t
	}
	if len(st.levels) != len(st.s.Tracks) {
		st.levels = make([]float32, len(st.s.Tracks))
	}
	for i, tr := range st.s.Tracks {
		target := min(tr.Level*2.2, 1)
		k := min(1, t*14)
		if target < st.levels[i] {
			k = min(1, t*5)
		}
		st.levels[i] += (target - st.levels[i]) * k
	}
	if st.s.Chord != st.chordWas {
		st.chordWas, st.chordAt = st.s.Chord, 1
	}
	st.chordAt = max(0, st.chordAt-t*2.2)
	return true
}
