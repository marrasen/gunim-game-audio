package plugsynth

import (
	"fmt"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// keys is a keyboard of two octaves that plays the sound: from C3 for
// a sound that plays notes, and from B1, General MIDI's drums from the
// kick up, for a kit. Through a host that is a [plug.Holder], a key
// sounds while the pointer holds it, and a drag across the keys plays
// each key it passes, letting the one before go, as a glissando; through
// any other [plug.Auditioner], a press plays a short note. A key lights
// as it sounds and fades after.
type keys struct {
	anim.Group
	a   plug.Auditioner
	low int
	// lit is how lit each key is, from low up, and hover the key under
	// the pointer, -1 for none.
	lit   []*anim.Float
	hover int
	size  geom.Size
	// pressing says the primary button holds the keyboard; held is the
	// key it holds, -1 for none, and note the note it plays, which names
	// it to the host.
	pressing bool
	held     int
	note     int
}

// keyCount is how many keys the keyboard has: two octaves and one.
const keyCount = 25

func newKeys(a plug.Auditioner) *keys {
	k := &keys{a: a, low: 48, hover: -1, held: -1}
	for range keyCount {
		f := anim.NewFloat(0)
		k.lit = append(k.lit, f)
		k.Add(f)
	}
	return k
}

// follow sets the keyboard's range for a patch of kind.
func (k *keys) follow(kind string) {
	k.low = 48
	if kind == "drums" {
		k.low = 35
	}
}

// black reports whether MIDI note n is a black key.
func black(n int) bool {
	switch n % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// whites returns how many white keys the keyboard has.
func (k *keys) whites() int {
	n := 0
	for i := range keyCount {
		if !black(k.low + i) {
			n++
		}
	}
	return n
}

// rect returns where key i stands.
func (k *keys) rect(i int) geom.Rect {
	ww := k.size.W / float32(k.whites())
	white := 0
	for j := range i {
		if !black(k.low + j) {
			white++
		}
	}
	if !black(k.low + i) {
		return geom.Rc(float32(white)*ww, 0, ww, k.size.H)
	}
	bw := ww * 0.62
	return geom.Rc(float32(white)*ww-bw/2, 0, bw, k.size.H*0.6)
}

// at returns the key at p, the black keys over the white, or -1.
func (k *keys) at(p geom.Point) int {
	for i := range keyCount {
		if black(k.low+i) && k.rect(i).Contains(p) {
			return i
		}
	}
	for i := range keyCount {
		if !black(k.low+i) && k.rect(i).Contains(p) {
			return i
		}
	}
	return -1
}

// Layout implements [gunim.Node].
func (k *keys) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	k.size = c.Max
	return c.Max
}

// Handle implements [gunim.Handler]: a press plays its key's note,
// held while the button is, and a drag moves it to each key it passes.
// Another button, pressed meanwhile, ends the press.
func (k *keys) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		k.hover = k.at(e.Pos)
		if k.pressing && k.hover != k.held {
			k.hold(k.hover, u)
		}
		u.Invalidate()
		return false
	case input.PointerLeave:
		k.hover = -1
		if k.pressing {
			k.hold(-1, u)
		}
		u.Invalidate()
		return false
	case input.PointerDown:
		if k.pressing {
			k.end(u)
			return true
		}
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := k.at(e.Pos)
		if i < 0 {
			return false
		}
		k.pressing = true
		k.hold(i, u)
		return true
	case input.PointerUp:
		if !k.pressing {
			return false
		}
		k.end(u)
		return true
	}
	return false
}

// end ends the press, letting its key go.
func (k *keys) end(u *gunim.UI) {
	k.hold(-1, u)
	k.pressing = false
}

// hold lets the key held go, and holds key i, -1 for none: it sounds
// while held, through a host that holds notes, and for half a second
// through one that plays them by length.
func (k *keys) hold(i int, u *gunim.UI) {
	h, holds := k.a.(plug.Holder)
	if k.held >= 0 {
		if holds {
			h.Release(k.note, u)
		}
		k.lit[k.held].Animate(0, anim.Spring{Response: 0.6, Damping: 1})
	}
	k.held = i
	if i >= 0 {
		k.note = k.low + i
		if holds {
			h.Hold(k.note, float64(k.note), 0.8, u)
		} else {
			k.a.Audition(float64(k.note), 0.8, 500*time.Millisecond, u)
		}
		k.lit[i].Jump(1)
	}
	u.Invalidate()
}

// Paint implements [gunim.Node]: ivory and ebony, a key lit in the
// theme's accent, the Cs named.
func (k *keys) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	for pass := range 2 {
		for i := range keyCount {
			n := k.low + i
			if black(n) != (pass == 1) {
				continue
			}
			r := k.rect(i)
			lit := k.lit[i].Value()
			hv := float32(0)
			if i == k.hover {
				hv = 1
			}
			if pass == 0 {
				body := r.Inset(geom.Insets{Left: 1, Right: 1})
				p.RRect(body, 4, paint.Solid(audioui.Mix(audioui.Faded(ink, 0.82+0.1*hv), accent, lit)))
				if n%12 == 0 {
					run := audioui.Shaped(fmt.Sprintf("C%d", n/12-1), 9, true, false)
					run.Paint(p, geom.Pt(r.Min.X+(r.Size().W-run.Advance)/2, r.Max.Y-run.Height()-4), audioui.Faded(audioui.Ground.Get(f.Theme), 0.6))
				}
				continue
			}
			p.ShadowRRect(r, 3, paint.Solid(audioui.Mix(audioui.Mix(audioui.Ground.Get(f.Theme), ink, 0.12*hv), accent, lit)),
				paint.Shadow{Blur: 4, Offset: geom.Pt(0, 2), Color: audioui.Faded(audioui.Ground.Get(f.Theme), 0.6)})
		}
	}
}

// Step implements [gunim.Animator].
func (k *keys) Step(dt time.Duration) bool { return k.Group.Step(dt) }

// Access implements [gunim.Accessible].
func (k *keys) Access() access.Info {
	return access.Info{Role: access.RoleGroup, Name: "Keys to hear the sound"}
}

// facts are short words on what patch p is made of, for chips under
// the sound's name: its kind, its oscillators, its filter, how many
// notes it plays at once, and the SoundFont program it stands in for.
func facts(p *synth.Patch) []string {
	var out []string
	switch p.Kind {
	case "drums":
		out = append(out, fmt.Sprintf("Drum kit of %d", len(p.Kit)))
	case "pluck":
		out = append(out, "Plucked string")
	default:
		waves := map[string]int{}
		var order []string
		for _, o := range p.Osc {
			if waves[o.Wave] == 0 {
				order = append(order, o.Wave)
			}
			waves[o.Wave] += max(o.Unison, 1)
		}
		var ws []string
		for _, w := range order {
			if waves[w] > 1 {
				ws = append(ws, fmt.Sprintf("%s ×%d", w, waves[w]))
			} else {
				ws = append(ws, w)
			}
		}
		if len(ws) > 0 {
			out = append(out, strings.Join(ws, " + "))
		}
		if p.Noise > 0 {
			out = append(out, "noise")
		}
	}
	if t := p.Filter.Type; t != "" && t != "none" && p.Kind != "drums" {
		f := t
		if p.Filter.Cutoff > 0 {
			f += " " + hzText(p.Filter.Cutoff)
		}
		out = append(out, f)
	}
	if p.Vowel != "" || p.Singer != "" {
		out = append(out, "sings")
	}
	switch {
	case p.Poly == 1:
		out = append(out, "mono")
	case p.Poly > 1:
		out = append(out, fmt.Sprintf("%d voices", p.Poly))
	}
	if p.SoundFont != "" {
		out = append(out, fmt.Sprintf("General MIDI %d", p.Program+1))
	}
	return out
}

// hzText writes a frequency to show, as 900 Hz or 1.5 kHz.
func hzText(hz float64) string {
	if hz >= 1000 {
		return fmt.Sprintf("%.3g kHz", hz/1000)
	}
	return fmt.Sprintf("%.0f Hz", hz)
}

// paintFacts draws words as chips from at, as many as fit in room.
func paintFacts(p *paint.Painter, f gunim.Frame, words []string, at geom.Point, room float32) {
	ink := widget.Ink.Get(f.Theme)
	x := at.X
	for _, w := range words {
		run := audioui.Shaped(w, 10, true, false)
		cw := run.Advance + 16
		if x+cw > at.X+room {
			return
		}
		r := geom.Rc(x, at.Y, cw, 20)
		p.RRect(r, 10, paint.Solid(audioui.Faded(ink, 0.07)))
		run.Paint(p, geom.Pt(x+8, at.Y+(20-run.Height())/2), audioui.Faded(ink, 0.7))
		x += cw + 6
	}
}
