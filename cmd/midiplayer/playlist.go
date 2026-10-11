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
)

// playlist is the files dropped and opened, a row each, the one playing
// lit, with bars that dance while it plays. A click plays a row, and
// its cross takes it off. New rows slide in from the right, rows glide
// to their places as others leave, and the light slides from the song
// that played to the next. The wheel scrolls it, gliding, and it
// scrolls by itself to keep the song playing in view.
type playlist struct {
	l *look
	// open says it shows, and closed that the user shut it, so new files
	// leave it shut.
	open, closed bool
	size         geom.Size
	scroll       float32
	over, down   int
	overX        bool
	// arrive is how far each row has slid in, by its index.
	arrive []float32
	// want is where the scroll glides to; lit is where the light of the
	// song playing is, in rows, gliding to it, and litFor the song it
	// glides to; at is where each row stands, in rows, gliding to its
	// place, by the song it shows.
	want   float32
	lit    float32
	litFor int
	at     map[string]float32
	// moving says something still glides, so the window draws on.
	moving bool
}

// Step keeps the window drawing while the playlist glides.
func (pl *playlist) Step(time.Duration) bool { return pl.moving || pl.want != pl.scroll }

const rowH = 46

func (pl *playlist) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	pl.size = c.Max
	return c.Max
}

// rowAt returns the row at pos, -1 for none, and whether pos is on its
// cross.
func (pl *playlist) rowAt(pos geom.Point) (int, bool) {
	y := pos.Y - 44 + pl.scroll
	if pos.Y < 44 || y < 0 {
		return -1, false
	}
	i := int(y / rowH)
	if i >= len(pl.l.s.Playlist) {
		return -1, false
	}
	return i, pos.X > pl.size.W-40
}

func (pl *playlist) Handle(e input.Event, u *gunim.UI) bool {
	if !pl.open {
		return false
	}
	switch e := e.(type) {
	case input.PointerMove:
		pl.over, pl.overX = pl.rowAt(e.Pos)
		u.Invalidate()
		return false
	case input.PointerLeave:
		pl.over = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		pl.down, _ = pl.rowAt(e.Pos)
		return pl.down >= 0
	case input.PointerUp:
		i, x := pl.rowAt(e.Pos)
		if i >= 0 && i == pl.down {
			if x {
				u.Send(pl, Removed{Index: i})
			} else {
				u.Send(pl, Picked{Index: i})
			}
		}
		pl.down = -1
	case input.Scroll:
		most := max(0, float32(len(pl.l.s.Playlist))*rowH-(pl.size.H-50))
		pl.want = min(max(pl.want-e.Delta.Y, 0), most)
		pl.moving = true
		u.Invalidate()
	default:
		return false
	}
	return true
}

func (pl *playlist) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	l := pl.l
	pal := l.pal
	s := l.s
	dt := float32(f.Delta.Seconds())
	if box.W < 20 {
		return
	}
	full := geom.Rect{Max: box.Point()}
	rad := 18 * pal.round
	p.ShadowRRect(full, rad, paint.Solid(alpha(pal.panel, 0.95)), paint.Shadow{Blur: 20, Offset: geom.Pt(0, 6), Color: alpha(color.NRGBA{A: 255}, 0.4)})
	p.RRectStroke(full, rad, paint.Fill{}, paint.Stroke{Width: 1, Color: alpha(pal.edge, 0.8)})
	head := shaped("Playlist", 15, true)
	head.Paint(p, geom.Pt(16, 14), pal.ink)
	count := shaped(fmt.Sprintf("%d", len(s.Playlist)), 12, false)
	count.Paint(p, geom.Pt(16+head.Advance+8, 17), pal.dim)
	for len(pl.arrive) < len(s.Playlist) {
		pl.arrive = append(pl.arrive, 0)
	}
	pl.arrive = pl.arrive[:len(s.Playlist)]
	end := p.Layer(paint.LayerOpts{Bounds: xyxy(0, 44, box.W, box.H-6), Opacity: 1, Clip: true, Fade: geom.Insets{Top: 6, Bottom: 14}})
	defer end()
	if len(s.Playlist) == 0 {
		msg := shaped("Drop files here to queue them", 12, false)
		msg.Paint(p, geom.Pt(16, 56), pal.dim)
		return
	}
	pl.glide(s, box, dt)
	// The light of the song playing, sliding from the last.
	if s.Current >= 0 && s.Current < len(s.Playlist) {
		y := 44 + pl.lit*rowH - pl.scroll
		d := float32(math.Abs(float64(pl.lit - float32(s.Current))))
		lr := xyxy(8, y+3, box.W-8, y+rowH-3)
		p.ShadowRRect(lr, 10*pal.round, paint.Solid(mix(pal.panel, pal.accent, 0.18)),
			paint.Shadow{Blur: 10 + 14*min(d, 1), Color: alpha(pal.accent, 0.15+0.25*min(d, 1))})
	}
	seen := map[string]int{}
	for i, it := range s.Playlist {
		pl.arrive[i] = min(1, pl.arrive[i]+dt*(3-float32(min(i, 10))*0.1))
		in := ease(pl.arrive[i])
		key := rowKey(it, seen)
		y := 44 + pl.at[key]*rowH - pl.scroll
		if y > box.H || y+rowH < 44 {
			continue
		}
		x := 30 * (1 - in)
		row := xyxy(8+x, y+3, box.W-8+x, y+rowH-3)
		cur := i == s.Current
		if i == pl.over && !cur {
			p.RRect(row, 10*pal.round, paint.Solid(alpha(lighten(pal.panel, 0.06), in)))
		}
		// The bars of the song playing dance; the others show their place.
		if cur {
			for b := range 3 {
				hh := float32(4)
				if s.Playing {
					hh = 4 + 10*(0.5+0.5*sinf(l.t*float64(7+b*3)+float64(b)))*(0.4+0.6*l.pulse+0.3)
				}
				bx := row.Min.X + 10 + float32(b)*5
				p.RRect(xyxy(bx, row.Max.Y-12-hh, bx+3, row.Max.Y-12), 1, paint.Solid(pal.accent))
			}
		} else {
			n := shaped(fmt.Sprint(i+1), 11, false)
			n.Paint(p, geom.Pt(row.Min.X+12, row.Min.Y+(row.Size().H-n.Height())/2), alpha(pal.dim, in))
		}
		ink := pal.ink
		if cur {
			ink = pal.accent
		}
		name := fit(it.Name, 13, cur, row.Size().W-90)
		name.Paint(p, geom.Pt(row.Min.X+34, row.Min.Y+6), alpha(ink, in))
		meta := shaped(clock(it.Length), 11, false)
		meta.Paint(p, geom.Pt(row.Min.X+34, row.Min.Y+23), alpha(pal.dim, in))
		if i == pl.over {
			c := alpha(pal.dim, 0.8)
			if pl.overX {
				c = rgb(0xff6b6b)
			}
			cx, cy := row.Max.X-20, row.Min.Y+row.Size().H/2
			p.Mask(icon.Stroke{Icon: icon.X, Width: 2, Progress: 1}, xyxy(cx-7, cy-7, cx+7, cy+7), c)
		}
	}
}

// rowKey names song it of the playlist, the same each frame while it
// stays, a second of the same name and file counted apart.
func rowKey(it Item, seen map[string]int) string {
	k := it.Path + "\x00" + it.Name
	seen[k]++
	return fmt.Sprintf("%s\x00%d", k, seen[k])
}

// glide moves the scroll, the light and the rows toward where they go:
// the scroll to keep the song playing in view as it changes.
func (pl *playlist) glide(s Player, box geom.Size, dt float32) {
	room := box.H - 50
	most := max(0, float32(len(s.Playlist))*rowH-room)
	if s.Current != pl.litFor {
		if pl.litFor < 0 || pl.litFor >= len(s.Playlist) {
			pl.lit = float32(max(s.Current, 0))
		}
		pl.litFor = s.Current
		if s.Current >= 0 {
			top := float32(s.Current) * rowH
			switch {
			case top < pl.want:
				pl.want = top - rowH/2
			case top+rowH > pl.want+room:
				pl.want = top + rowH*1.5 - room
			}
		}
	}
	pl.want = min(max(pl.want, 0), most)
	k := min(1, dt*10)
	pl.scroll += (pl.want - pl.scroll) * k
	if math.Abs(float64(pl.want-pl.scroll)) < 0.3 {
		pl.scroll = pl.want
	}
	if s.Current >= 0 {
		pl.lit += (float32(s.Current) - pl.lit) * min(1, dt*9)
	}
	if pl.at == nil {
		pl.at = map[string]float32{}
	}
	seen := map[string]int{}
	keep := map[string]float32{}
	for i, it := range s.Playlist {
		key := rowKey(it, seen)
		y, ok := pl.at[key]
		if !ok {
			y = float32(i)
		}
		y += (float32(i) - y) * min(1, dt*12)
		keep[key] = y
	}
	pl.at = keep
	pl.moving = pl.scroll != pl.want || math.Abs(float64(pl.lit-float32(max(s.Current, 0)))) > 0.002
	for key, y := range keep {
		_ = key
		if y != float32(int(y)) && math.Abs(float64(y-float32(math.Round(float64(y))))) > 0.002 {
			pl.moving = true
		}
	}
	if !pl.moving && s.Current >= 0 {
		pl.lit = float32(s.Current)
	}
}
