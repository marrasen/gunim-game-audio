package plugsynth

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
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
// as it sounds and fades after: through a [plug.KeysHost], each key the
// sound plays lights too, as the song plays it or a MIDI keyboard does.
// The buttons at its sides, or Z and X, shift it an octave down or up,
// and the keys slide across.
type keys struct {
	anim.Group
	a  plug.Auditioner
	kh plug.KeysHost
	// base is the lowest key for the sound's kind, shift the octaves
	// the keyboard stands from it, and low the lowest key it shows.
	base, shift, low int
	// slide is the white key at the left edge, as the keys slide there.
	slide *anim.Float
	// lit is how lit each key is, by MIDI note; heard are the keys the
	// host says sound; and hover is the key under the pointer, -1 for
	// none.
	lit   [128]*anim.Float
	heard plug.Keys
	hover int
	down  *widget.IconButton
	up    *widget.IconButton
	size  geom.Size
	// pressing says the primary button holds the keyboard; held is the
	// key it holds, -1 for none, which names its note to the host.
	pressing bool
	held     int
}

// keyCount is how many keys the keyboard has: two octaves and one.
// shiftW is how wide the octave buttons stand at its sides.
const (
	keyCount = 25
	shiftW   = 28
)

func newKeys(a plug.Auditioner) *keys {
	k := &keys{a: a, base: 48, low: 48, hover: -1, held: -1}
	k.kh, _ = a.(plug.KeysHost)
	k.slide = anim.NewFloat(float32(whiteOf(k.low)))
	k.Add(k.slide)
	for n := range k.lit {
		k.lit[n] = anim.NewFloat(0)
		k.Add(k.lit[n])
	}
	k.down = widget.NewIconButton(icon.ChevronLeft, "An octave down (Z)")
	k.up = widget.NewIconButton(icon.ChevronRight, "An octave up (X)")
	k.down.OnClick = func(u *gunim.UI) gunim.Intent { k.shiftBy(-1, u); return nil }
	k.up.OnClick = func(u *gunim.UI) gunim.Intent { k.shiftBy(1, u); return nil }
	k.enable()
	return k
}

// follow sets the keyboard's range for a patch of kind: a kind of
// another range starts it at its own octave.
func (k *keys) follow(kind string) {
	base := 48
	if kind == "drums" {
		base = 35
	}
	if base == k.base {
		return
	}
	k.base, k.shift = base, 0
	k.low = base
	k.slide.Jump(float32(whiteOf(k.low)))
	k.enable()
}

// canShift reports whether the keyboard may shift by d octaves and stay
// within MIDI's keys.
func (k *keys) canShift(d int) bool {
	low := k.base + 12*(k.shift+d)
	return low >= 0 && low+keyCount-1 <= 127
}

// enable greys the octave buttons where the keyboard has no octave to
// go to.
func (k *keys) enable() {
	k.down.Disabled = !k.canShift(-1)
	k.up.Disabled = !k.canShift(1)
}

// shiftBy shifts the keyboard d octaves, where it may: the keys slide
// across, and a key held lets its note go; the press goes on, and holds
// the key the pointer moves onto next.
func (k *keys) shiftBy(d int, u *gunim.UI) {
	if !k.canShift(d) {
		return
	}
	k.shift += d
	k.low = k.base + 12*k.shift
	k.enable()
	k.slide.Animate(float32(whiteOf(k.low)), widget.Settle.Get(u.Theme()))
	if k.pressing {
		k.hold(-1, u)
	}
	u.Invalidate()
}

// rangeText writes the keys shown, as "C3 to C5".
func (k *keys) rangeText() string {
	return keyName(k.low) + " to " + keyName(k.low+keyCount-1)
}

// keyName names MIDI note n, as C4 for 60.
func keyName(n int) string {
	return [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[n%12] + strconv.Itoa(n/12-1)
}

// black reports whether MIDI note n is a black key.
func black(n int) bool {
	switch n % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// whiteOf returns how many white keys lie below MIDI note n: for a
// black key, the white key it stands after, plus one.
func whiteOf(n int) int {
	return n/12*7 + [12]int{0, 1, 1, 2, 2, 3, 4, 4, 5, 5, 6, 6}[n%12]
}

// whites returns how many white keys the keyboard shows.
func (k *keys) whites() int {
	n := 0
	for i := range keyCount {
		if !black(k.low + i) {
			n++
		}
	}
	return n
}

// board is where the keys stand, between the octave buttons.
func (k *keys) board() geom.Rect {
	return geom.Rc(shiftW+4, 0, max(k.size.W-2*(shiftW+4), 0), k.size.H)
}

// noteRect returns where MIDI note n stands, as the keys slide.
func (k *keys) noteRect(n int) geom.Rect {
	b := k.board()
	ww := b.Size().W / float32(k.whites())
	x := b.Min.X + (float32(whiteOf(n))-k.slide.Value())*ww
	if !black(n) {
		return geom.Rc(x, 0, ww, k.size.H)
	}
	bw := ww * 0.62
	return geom.Rc(x-bw/2, 0, bw, k.size.H*0.6)
}

// rect returns where key i, from the lowest shown, stands.
func (k *keys) rect(i int) geom.Rect { return k.noteRect(k.low + i) }

// at returns the MIDI note of the key at p, the black keys over the
// white, or -1.
func (k *keys) at(p geom.Point) int {
	if !k.board().Contains(p) {
		return -1
	}
	for _, want := range []bool{true, false} {
		for n := max(k.low-12, 0); n <= min(k.low+keyCount+12, 127); n++ {
			if black(n) == want && k.noteRect(n).Contains(p) {
				return n
			}
		}
	}
	return -1
}

// Children implements [gunim.Composite]: the octave buttons.
func (k *keys) Children() []gunim.Node { return []gunim.Node{k.down, k.up} }

// Layout implements [gunim.Node]: the buttons at the sides, in the
// middle of the height.
func (k *keys) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k.size = c.Max
	y := (k.size.H - shiftW) / 2
	kids.At(0).Layout(gunim.Tight(geom.Sz(shiftW, shiftW)))
	kids.At(0).Place(geom.Pt(0, y))
	kids.At(1).Layout(gunim.Tight(geom.Sz(shiftW, shiftW)))
	kids.At(1).Place(geom.Pt(k.size.W-shiftW, y))
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
		n := k.at(e.Pos)
		if n < 0 {
			return false
		}
		k.pressing = true
		k.hold(n, u)
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

// hold lets the key held go, and holds MIDI note n, -1 for none: it
// sounds while held, through a host that holds notes, and for half a
// second through one that plays them by length.
func (k *keys) hold(n int, u *gunim.UI) {
	h, holds := k.a.(plug.Holder)
	if was := k.held; was >= 0 {
		if holds {
			h.Release(was, u)
		}
		k.held = -1
		k.light(was)
	}
	k.held = n
	if n >= 0 {
		if holds {
			h.Hold(n, float64(n), 0.8, u)
		} else {
			k.a.Audition(float64(n), 0.8, 500*time.Millisecond, u)
		}
		k.light(n)
	}
	u.Invalidate()
}

// light lights note n while the pointer holds it or the sound plays
// it, and fades it after.
func (k *keys) light(n int) {
	if n == k.held || k.heard.Has(n) {
		k.lit[n].Jump(1)
		return
	}
	if k.lit[n].Target() != 0 {
		k.lit[n].Animate(0, anim.Spring{Response: 0.6, Damping: 1})
	}
}

// listen lights the keys the host says sound now, and fades those it
// let go.
func (k *keys) listen() {
	if k.kh == nil {
		return
	}
	now := k.kh.Keys()
	if now == k.heard {
		return
	}
	was := k.heard
	k.heard = now
	for w := range 2 {
		for d := was[w] ^ now[w]; d != 0; d &= d - 1 {
			k.light(w*64 + bits.TrailingZeros64(d))
		}
	}
}

// Paint implements [gunim.Node]: ivory and ebony, a key lit in the
// theme's accent, the Cs named; then the octave buttons.
func (k *keys) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	ink, accent, ground := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme), audioui.Ground.Get(f.Theme)
	b := k.board()
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: b, Clip: true, Opacity: 1})()
		from, to := max(k.low-12, 0), min(k.low+keyCount+12, 127)
		for pass := range 2 {
			for n := from; n <= to; n++ {
				if black(n) != (pass == 1) {
					continue
				}
				r := k.noteRect(n)
				if r.Max.X < b.Min.X || r.Min.X > b.Max.X {
					continue
				}
				lit := k.lit[n].Value()
				hv := float32(0)
				if n == k.hover {
					hv = 1
				}
				if pass == 0 {
					body := r.Inset(geom.Insets{Left: 1, Right: 1})
					p.RRect(body, 4, paint.Solid(audioui.Mix(audioui.Faded(ink, 0.82+0.1*hv), accent, lit)))
					if n%12 == 0 {
						run := audioui.Shaped(keyName(n), 9, true, false)
						run.Paint(p, geom.Pt(r.Min.X+(r.Size().W-run.Advance)/2, r.Max.Y-run.Height()-4), audioui.Faded(ground, 0.6))
					}
					continue
				}
				p.ShadowRRect(r, 3, paint.Solid(audioui.Mix(audioui.Mix(ground, ink, 0.12*hv), accent, lit)),
					paint.Shadow{Blur: 4, Offset: geom.Pt(0, 2), Color: audioui.Faded(ground, 0.6)})
			}
		}
	}()
	for c := range kids.All {
		c.Paint(p)
	}
}

// Step implements [gunim.Animator]: the keys light as the sound plays
// them, and keep stepping while any sounds, to see it let go.
func (k *keys) Step(dt time.Duration) bool {
	k.listen()
	moving := k.Group.Step(dt)
	return moving || k.heard != plug.Keys{}
}

// Access implements [gunim.Accessible].
func (k *keys) Access() access.Info {
	return access.Info{Role: access.RoleGroup, Name: "Keys to hear the sound", Value: k.rangeText()}
}

// facts are short words on what patch p is made of, for chips under
// the sound's name: its kind, its oscillators, its filter, how many
// notes it plays at once, and the SoundFont program it stands in for:
// the bank it plays, or, where the bank is missing, the General MIDI
// sound it plays in its place.
func facts(p *synth.Patch) []string {
	var out []string
	if sampled(p) {
		f, ok := fontNamed(p.SoundFont)
		switch {
		case !ok && (p.Bank == synth.DrumBank || p.FontKit):
			return []string{"Recorded", "bank missing", "General MIDI kit"}
		case !ok:
			return []string{"Recorded", "bank missing", fmt.Sprintf("General MIDI %d", p.Program+1)}
		case f.Group == "" && p.Bank != synth.DrumBank:
			return []string{"Recorded", f.Title, fmt.Sprintf("General MIDI %d", p.Program+1)}
		}
		return []string{"Recorded", f.Title}
	}
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

// sampled reports whether p plays a SoundFont bank's samples, as the
// bank's own sound: a patch of nothing but the bank.
func sampled(p *synth.Patch) bool {
	return p.SoundFont != "" && p.Kind == "" && len(p.Osc) == 0 && p.Noise == 0
}
