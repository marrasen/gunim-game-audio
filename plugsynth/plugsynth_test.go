package plugsynth

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// host is a host for tests, which keeps the parameters it is sent.
type host struct{ values map[uint32]float64 }

func (h *host) Param(id uint32) float64               { return h.values[id] }
func (h *host) Begin(uint32, *gunim.UI)               {}
func (h *host) Set(id uint32, v float64, _ *gunim.UI) { h.values[id] = v }
func (h *host) End(uint32, *gunim.UI)                 {}
func (h *host) BeginEdit(string, *gunim.UI)           {}
func (h *host) Changed(*gunim.UI)                     {}
func (h *host) EndEdit(*gunim.UI)                     {}
func (h *host) Sound() (in, out plug.Sound)           { return nil, nil }
func (h *host) Traced(uint32, int64) (float32, bool)  { return 0, false }

func newSynth(t *testing.T) (*plugin, *host) {
	t.Helper()
	d, ok := plug.Lookup(Descriptor.ID)
	if !ok {
		t.Fatal("the synth did not register itself")
	}
	if !d.Instrument() || !d.Notes {
		t.Fatalf("the synth registers as %q, taking notes %v: not an instrument", d.Kind, d.Notes)
	}
	h := &host{values: map[uint32]float64{}}
	for _, p := range d.Params {
		h.values[p.ID] = p.Default
	}
	p, ok := d.New(h).(*plugin)
	if !ok {
		t.Fatal("the synth's New makes another type")
	}
	return p, h
}

// params are the synth's parameters, sound at its default, but the
// sound, which is s.
func params(s int) []plug.Param {
	out := append([]plug.Param(nil), Descriptor.Params...)
	out[0].Default = float64(s)
	return out
}

// note is a note's NoteOn at frame on and its NoteOff at off.
func note(id int32, pitch float64, on, off int) []plug.Event {
	return []plug.Event{
		{Frame: on, Kind: plug.NoteOn, Note: id, Pitch: pitch, Velocity: 0.9},
		{Frame: off, Kind: plug.NoteOff, Note: id},
	}
}

func rms(x []float32) float64 {
	var sum float64
	for _, s := range x {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(max(len(x), 1)))
}

// hz returns how high x, at rate, sounds: the period, from 1 ms to 20,
// over which it best matches itself.
func hz(x []float32, rate float64) float64 {
	lo, hi := int(rate/1000), int(rate/50)
	sums := make([]float64, hi+2)
	best := 0.0
	for lag := lo - 1; lag <= hi+1; lag++ {
		for i := lag; i < len(x); i++ {
			sums[lag] += float64(x[i]) * float64(x[i-lag])
		}
		sums[lag] /= float64(len(x) - lag)
		best = max(best, sums[lag])
	}
	// The first peak near the best, so a period twice as long loses,
	// placed between lags by the parabola through it.
	for lag := lo; lag <= hi; lag++ {
		a, b, c := sums[lag-1], sums[lag], sums[lag+1]
		if b >= a && b >= c && b >= 0.9*best {
			d := 0.0
			if den := a - 2*b + c; den != 0 {
				d = 0.5 * (a - c) / den
			}
			return float64(rate) / (float64(lag) + d)
		}
	}
	return 0
}

func TestEverySoundPlaysANote(t *testing.T) {
	for i, s := range Sounds {
		p, _ := newSynth(t)
		pitch := 57.0
		if s.Short == "Drums" {
			pitch = 38
		}
		setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
		out := plugtest.Play(t, p.Processor(), setup, 48000*3, params(i), note(1, pitch, 100, 48000)...)
		for c, ch := range out {
			for f, v := range ch {
				if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 2 {
					t.Fatalf("%s: channel %d, frame %d, is %v", s.Short, c, f, v)
				}
			}
		}
		if got := rms(out[0][:100]); got != 0 {
			t.Errorf("%s: before its note, the synth sounds, at %.4f", s.Short, got)
		}
		if got := rms(out[0][2000:12000]); got < 0.005 {
			t.Errorf("%s: its note sounds at %.4f", s.Short, got)
		}
		if got := rms(out[0][len(out[0])-4800:]); got > 1e-3 {
			t.Errorf("%s: 2 s after its NoteOff, the note sounds on, at %.4f", s.Short, got)
		}
	}
}

func TestTheSynthPlaysInTuneAtTheHostsRate(t *testing.T) {
	for _, rate := range []float64{48000, 44100, 96000} {
		p, _ := newSynth(t)
		setup := plug.Setup{Rate: rate, MaxBlock: 256, Channels: 2}
		n := int(rate)
		out := plugtest.Play(t, p.Processor(), setup, n, params(3), note(1, 57, 0, n-1)...)
		// The lead's lowest saws play an octave down, at 110 Hz.
		if got := hz(out[0][n/2:], rate); math.Abs(got-110) > 2 {
			t.Errorf("at %.0f Hz, A3 on the lead sounds at %.1f Hz, not 110", rate, got)
		}
	}
}

func TestAPedalAndABendReachTheSynth(t *testing.T) {
	p, _ := newSynth(t)
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	events := append(note(1, 57, 0, 12000),
		plug.Event{Frame: 6000, Kind: plug.Pedal, Group: 0, Value: 1},
		plug.Event{Frame: 24000, Kind: plug.Expression, Note: 1, Expr: plug.Bend, Value: 12},
	)
	out := plugtest.Play(t, p.Processor(), setup, 48000, params(3), events...)
	// The lead's lowest saws play an octave down, at 220 Hz bent.
	if got := hz(out[0][36000:], 48000); math.Abs(got-220) > 4 {
		t.Errorf("held by the pedal and bent up an octave, A3 on the lead sounds at %.1f Hz, not 220", got)
	}
}

func TestResetEndsTheNotes(t *testing.T) {
	p, _ := newSynth(t)
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	pr := p.Processor()
	plugtest.Play(t, pr, setup, 4800, params(0),
		plug.Event{Kind: plug.Pedal, Value: 1},
		plug.Event{Kind: plug.NoteOn, Note: 1, Pitch: 60, Velocity: 1})
	pr.Reset()
	l, r := make([]float32, 512), make([]float32, 512)
	pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512})
	if rms(l) != 0 || rms(r) != 0 {
		t.Error("after Reset, the note sounds on")
	}
	// The pedal lifted with the Reset: a note let go after it ends.
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 512, Events: []plug.Event{
		{Kind: plug.NoteOn, Note: 2, Pitch: 60, Velocity: 1}, {Frame: 256, Kind: plug.NoteOff, Note: 2},
	}}
	pr.Process(b)
	b.Events = nil
	for range 200 {
		pr.Process(b)
	}
	if rms(l) > 1e-4 {
		t.Errorf("a note let go after Reset rings on at %.5f, held by the pedal from before", rms(l))
	}
}

func TestProcessAllocatesNothing(t *testing.T) {
	p, _ := newSynth(t)
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	l, r := make([]float32, 256), make([]float32, 256)
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 256, Events: make([]plug.Event, 4), Changes: make([]plug.Change, 1)}
	n := plugtest.AllocsUnder(t, "plugsynth.(*processor).Process", func() {
		for i := range 400 {
			id := int32(i)
			b.Events[0] = plug.Event{Frame: 10, Kind: plug.NoteOn, Note: id, Pitch: 48 + float64(i%24), Velocity: 0.8, Group: id % 2}
			b.Events[1] = plug.Event{Frame: 20, Kind: plug.Expression, Note: id, Expr: plug.Bend, Value: 0.5}
			b.Events[2] = plug.Event{Frame: 30, Kind: plug.Pedal, Group: id % 2, Value: float64(i % 2)}
			b.Events[3] = plug.Event{Frame: 200, Kind: plug.NoteOff, Note: id - 3}
			b.Changes[0] = plug.Change{Frame: 100, ID: SoundID, Value: float64(i / 60 % len(Sounds))}
			pr.Process(b)
		}
	})
	if n != 0 {
		t.Errorf("400 blocks allocate %d times", n)
	}
}

func TestThePatchesPickEachSound(t *testing.T) {
	if len(Descriptor.Patches) != len(Sounds) {
		t.Fatalf("%d patches for %d sounds", len(Descriptor.Patches), len(Sounds))
	}
	for i, pt := range Descriptor.Patches {
		if pt.Params[SoundID] != float64(i) || pt.Name != Sounds[i].Preset {
			t.Errorf("patch %q picks sound %v", pt.Name, pt.Params[SoundID])
		}
	}
}

func TestTheViewsShowAndFollow(t *testing.T) {
	p, h := newSynth(t)
	h.values[SoundID] = 2
	c, ok := p.View(plug.Chip, h).(*chip)
	if !ok || c.label.Text != "Bass" {
		t.Fatal("the chip shows another sound than the host's Bass")
	}
	c.Follow(SoundID, 5, nil)
	if c.label.Text != "Drums" {
		t.Errorf("the chip follows to %q, not Drums", c.label.Text)
	}
	for _, size := range []plug.Size{plug.Panel, plug.Full} {
		pn, ok := p.View(size, h).(*panel)
		if !ok || pn.pick.Selected() != 2 {
			t.Fatalf("view %d picks another sound than the host's Bass", size)
		}
		// A pick on the selector goes to the host.
		pn.pick.OnPick(4, nil)
		if h.values[SoundID] != 4 {
			t.Errorf("view %d: a pick of Keys sets the sound to %v", size, h.values[SoundID])
		}
		for i, r := range boundsOf(t, pn, geom.Sz(900, 420), pn.sections) {
			if r.Empty() || r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 900 || r.Max.Y > 420 {
				t.Errorf("view %d: section %s is at %v, outside the view", size, pn.sections[i].Name, r)
			}
		}
		h.values[SoundID] = 2
	}
}

// boundsOf lays pn out at size, paints it, and returns where each of
// sections is.
func boundsOf(t *testing.T, pn *panel, size geom.Size, sections []*plugui.Section) []geom.Rect {
	t.Helper()
	w := gunimtest.New(t, size, nil)
	gunim.RegisterView(w, "synth", func(struct{}) gunim.Node { return pn }, nil)
	if err := w.Client().Mount(gunim.Root, "synth", "synth", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	out := make([]geom.Rect, len(sections))
	gunim.RegisterPatch(w, "synth", func(_ gunim.Node, _ struct{}, u *gunim.UI) {
		for i, s := range sections {
			out[i], _ = u.Bounds(s)
		}
	})
	if err := w.Client().Patch("synth", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	return out
}

// onset returns the first frame of x that sounds, or -1.
func onset(x []float32) int {
	for i, v := range x {
		if v != 0 {
			return i
		}
	}
	return -1
}

// drum is a bass drum's NoteOn at frame f: it sounds from its first
// frame.
func drum(id int32, f int) plug.Event {
	return plug.Event{Frame: f, Kind: plug.NoteOn, Note: id, Pitch: 36, Velocity: 1}
}

func TestANoteSoundsLatencyFramesLate(t *testing.T) {
	p, _ := newSynth(t)
	pr := p.Processor()
	setup := plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2}
	// Right after Prepare, at the first frame, and at frames inside and
	// at the start of later blocks.
	for _, f := range []int{0, 1, 77, 128, 300} {
		out := plugtest.Play(t, pr, setup, 1024, params(5), drum(1, f))
		if got := onset(out[0]); got != f+pr.Latency() {
			t.Errorf("a drum at frame %d sounds from frame %d, not %d, Latency %d", f, got, f+pr.Latency(), pr.Latency())
		}
	}
	// Right after a Reset, too.
	for _, f := range []int{0, 5} {
		pr.Reset()
		l, r := make([]float32, 128), make([]float32, 128)
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 128, Events: []plug.Event{drum(2, f)}})
		if got := onset(l); got != f+pr.Latency() {
			t.Errorf("after Reset, a drum at frame %d sounds from frame %d, not %d", f, got, f+pr.Latency())
		}
	}
}

func TestTheDelayIsTheSameAtOtherRates(t *testing.T) {
	// At 44.1 kHz each 147 frames are 160 of the engine's, and at 96 kHz
	// each 2 are 1: a note moved by as many sounds the same, moved.
	for _, c := range []struct {
		rate  float64
		every int
		delay int
	}{{44100, 147, 2}, {96000, 2, 5}} {
		setup := plug.Setup{Rate: c.rate, MaxBlock: 100, Channels: 2}
		// Each on a synth of its own, so its drum's noise starts alike.
		p, _ := newSynth(t)
		pr := p.Processor()
		a := plugtest.Play(t, pr, setup, 4000, params(5), drum(1, c.every))
		q, _ := newSynth(t)
		b := plugtest.Play(t, q.Processor(), setup, 4000, params(5), drum(1, 7*c.every))
		if pr.Latency() != c.delay {
			t.Errorf("at %.0f Hz, Latency is %d, want %d", c.rate, pr.Latency(), c.delay)
		}
		shift := 6 * c.every
		for i := range 4000 - shift {
			if a[0][i] != b[0][i+shift] {
				t.Errorf("at %.0f Hz, a drum moved %d frames sounds otherwise from frame %d", c.rate, shift, i)
				break
			}
		}
		if got := onset(a[0]); got < c.every+c.delay-3 || got > c.every+c.delay {
			t.Errorf("at %.0f Hz, a drum at %d starts sounding at %d, Latency %d", c.rate, c.every, got, c.delay)
		}
	}
}

func TestTheTailIsAsLongAsTheRelease(t *testing.T) {
	p, _ := newSynth(t)
	pr := p.Processor()
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	ps := params(0)
	ps[ReleaseID-1].Default = 8
	n := 48000 * 14
	out := plugtest.Play(t, pr, setup, n, ps, note(1, 57, 0, 24000)...)
	tail := pr.Tail()
	if tail < 48000*8 {
		t.Fatalf("with the release stretched 8 times, the tail is %d frames", tail)
	}
	if rms(out[0][24000+tail/2:24000+tail/2+4800]) < 1e-4 {
		t.Errorf("halfway through the tail of %d frames, the note is silent", tail)
	}
	if end := 24000 + tail; end+4800 <= n && rms(out[0][end:end+4800]) > 1e-3 {
		t.Errorf("after the tail of %d frames, the note sounds at %.4f", tail, rms(out[0][end:end+4800]))
	}
}

func TestTheKnobsTurnEverySound(t *testing.T) {
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	level := func(changes ...plug.Change) float64 {
		p, _ := newSynth(t)
		pr := p.Processor()
		pr.Prepare(setup)
		l, r := make([]float32, 512), make([]float32, 512)
		first := &plug.Block{Out: [][]float32{l, r}, Frames: 512, Changes: changes,
			Events: []plug.Event{{Frame: 10, Kind: plug.NoteOn, Note: 1, Pitch: 57, Velocity: 1}}}
		pr.Process(first)
		sum := 0.0
		for range 40 {
			pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512})
			sum += rms(l)
		}
		return sum
	}
	lead := level(plug.Change{ID: SoundID, Value: 3})
	// The gain turned before the sound changes, and after.
	before := level(plug.Change{ID: GainID, Value: -20}, plug.Change{ID: SoundID, Value: 3})
	after := level(plug.Change{ID: SoundID, Value: 3}, plug.Change{ID: GainID, Value: -20})
	for name, got := range map[string]float64{"before": before, "after": after} {
		if r := got / lead; math.Abs(r-0.1) > 0.01 {
			t.Errorf("-20 dB of gain turned %s the sound changed turns the lead by %.3f, not 0.1", name, r)
		}
	}
}

func TestPressureAndModLeaveThePitchBe(t *testing.T) {
	for _, e := range []plug.ExprKind{plug.Pressure, plug.Mod} {
		p, _ := newSynth(t)
		setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
		events := append(note(1, 57, 0, 47999), plug.Event{Frame: 100, Kind: plug.Expression, Note: 1, Expr: e, Value: 1})
		out := plugtest.Play(t, p.Processor(), setup, 48000, params(3), events...)
		if got := hz(out[0][24000:], 48000); math.Abs(got-110) > 2 {
			t.Errorf("expression %d moves the lead to %.1f Hz", e, got)
		}
	}
}

// knob returns the panel's knob of parameter id.
func knob(pn *panel, id uint32) *plugui.Knob {
	for _, s := range pn.sections {
		for _, k := range s.Knobs {
			if k.Param().ID == id {
				return k
			}
		}
	}
	return nil
}

func TestTheKnobsASoundIgnoresAreGrey(t *testing.T) {
	p, h := newSynth(t)
	pn, ok := p.View(plug.Panel, h).(*panel)
	if !ok {
		t.Fatal("the panel is of another type")
	}
	for i, s := range Sounds {
		pn.Follow(SoundID, float64(i), nil)
		filter := s.Short == "Pad" || s.Short == "Bass" || s.Short == "Lead"
		if knob(pn, CutoffID).Disabled == filter || knob(pn, ResID).Disabled == filter {
			t.Errorf("%s: the filter's knobs are grey %v, though it has a filter %v", s.Short, knob(pn, CutoffID).Disabled, filter)
		}
		if env := s.Short != "Drums"; knob(pn, ReleaseID).Disabled == env || knob(pn, AttackID).Disabled == env {
			t.Errorf("%s: the envelope's knobs are grey %v", s.Short, knob(pn, ReleaseID).Disabled)
		}
		if knob(pn, GainID).Disabled {
			t.Errorf("%s: the gain is grey", s.Short)
		}
	}
	pn.pick.OnPick(1, nil)
	if !knob(pn, CutoffID).Disabled {
		t.Error("a pick of the pluck leaves its cutoff lit")
	}
}

func TestTheSoundIsTheSameInAnyBlockSize(t *testing.T) {
	for _, s := range []int{0, 2} {
		var want []float32
		for _, block := range []int{256, 1, 37} {
			p, _ := newSynth(t)
			setup := plug.Setup{Rate: 48000, MaxBlock: block, Channels: 2}
			events := append(note(1, 45, 10, 30000), note(2, 52, 9000, 20000)...)
			out := plugtest.Play(t, p.Processor(), setup, 40000, params(s), events...)[0]
			if want == nil {
				want = out
				continue
			}
			for i := range want {
				if out[i] != want[i] {
					t.Errorf("%s in blocks of %d differs from blocks of 256 from frame %d", Sounds[s].Short, block, i)
					break
				}
			}
		}
	}
}
