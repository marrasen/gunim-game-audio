package plugsynth

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// host is a host for tests, which keeps the parameters it is sent,
// and logs the edits and the notes auditioned.
type host struct {
	values map[uint32]float64
	log    []string
	heard  []float64
}

func (h *host) Param(id uint32) float64               { return h.values[id] }
func (h *host) Begin(uint32, *gunim.UI)               {}
func (h *host) Set(id uint32, v float64, _ *gunim.UI) { h.values[id] = v }
func (h *host) End(uint32, *gunim.UI)                 {}
func (h *host) BeginEdit(name string, _ *gunim.UI)    { h.log = append(h.log, "begin "+name) }
func (h *host) Changed(*gunim.UI)                     { h.log = append(h.log, "changed") }
func (h *host) EndEdit(*gunim.UI)                     { h.log = append(h.log, "end") }
func (h *host) Sound() (in, out plug.Sound)           { return nil, nil }
func (h *host) Traced(uint32, int64) (float32, bool)  { return 0, false }

// Audition implements [plug.Auditioner].
func (h *host) Audition(pitch, _ float64, _ time.Duration, _ *gunim.UI) {
	h.heard = append(h.heard, pitch)
}

func newSynth(t testing.TB) (*plugin, *host) {
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

// synthOf returns a synth playing the sound named name.
func synthOf(t testing.TB, name string) *plugin {
	t.Helper()
	p, _ := newSynth(t)
	play(t, p, name)
	return p
}

// play has p play the sound named name.
func play(t testing.TB, p *plugin, name string) {
	t.Helper()
	s, ok := Named(name)
	if !ok {
		t.Fatalf("no sound is named %s", name)
	}
	if err := p.SetState(s.State()); err != nil {
		t.Fatal(err)
	}
}

// params are the synth's parameters, each at its default.
func params() []plug.Param { return append([]plug.Param(nil), Descriptor.Params...) }

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

// kitNotes are the drums a kit's test plays: a kick, a snare and a
// closed hat, by General MIDI's notes.
var kitNotes = []float64{36, 38, 42}

// sound plays one note, or a kit's three drums, of a fresh synth playing
// s, for a second and a half, and returns the left channel and how loud
// it sounds while the note does.
func sound(t *testing.T, s Sound) (out [][]float32, level float64) {
	t.Helper()
	p, _ := newSynth(t)
	if err := p.SetState(s.State()); err != nil {
		t.Fatalf("%s/%s/%s: %v", s.Bank, s.Group, s.Name, err)
	}
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	var events []plug.Event
	if s.patch.Kind == "drums" {
		for i, k := range kitNotes {
			events = append(events, plug.Event{Frame: 100 + 4800*i, Kind: plug.NoteOn, Note: int32(i + 1), Pitch: k, Velocity: 0.9})
		}
	} else {
		events = note(1, 60, 100, 36000)
	}
	out = plugtest.Play(t, p.Processor(), setup, 72000, params(), events...)
	return out, rms(out[0][100:36000]) + rms(out[1][100:36000])
}

func TestEverySoundLoadsAndSounds(t *testing.T) {
	ss := Sounds()
	banks := map[string]int{}
	for _, s := range ss {
		banks[s.Bank]++
	}
	// 69 presets; 128 programs and 3 kits; 214 patches of the songs.
	if banks[Library] != 69 || banks[GeneralMIDI] != 131 || banks[Songs] != 214 {
		t.Fatalf("the banks hold %v sounds", banks)
	}
	for _, s := range ss {
		out, level := sound(t, s)
		for c, ch := range out {
			for f, v := range ch {
				if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 4 {
					t.Fatalf("%s/%s/%s: channel %d, frame %d, is %v", s.Bank, s.Group, s.Name, c, f, v)
				}
			}
		}
		if got := rms(out[0][:100]); got != 0 {
			t.Errorf("%s/%s/%s: before its note, the synth sounds, at %.4f", s.Bank, s.Group, s.Name, got)
		}
		if level < 0.002 {
			t.Errorf("%s/%s/%s: its note sounds at %.5f", s.Bank, s.Group, s.Name, level)
		}
	}
}

func TestEveryGroupIsListedOnce(t *testing.T) {
	if got := Groups(GeneralMIDI); len(got) != 17 || got[0] != "Piano" || got[16] != GMDrums {
		t.Errorf("General MIDI's groups are %q", got)
	}
	if got := len(Groups(Library)); got != 9 {
		t.Errorf("the library has %d groups, not its 9 categories", got)
	}
	if got := len(Groups(Songs)); got != 28 {
		t.Errorf("%d songs play patches, not 28", got)
	}
	seen := map[[3]string]bool{}
	for _, s := range Sounds() {
		k := [3]string{s.Bank, s.Group, s.Name}
		if seen[k] {
			t.Errorf("%v is listed twice", k)
		}
		seen[k] = true
		if f, ok := Find(s.Bank, s.Group, s.Name); !ok || f.Name != s.Name {
			t.Errorf("Find misses %v", k)
		}
	}
}

func TestTheStateRoundTrips(t *testing.T) {
	for _, s := range Sounds() {
		p, _ := newSynth(t)
		if err := p.SetState(s.State()); err != nil {
			t.Fatal(err)
		}
		saved := p.State()
		if !bytes.Equal(saved, s.State()) {
			t.Errorf("%s: the state comes back otherwise", s.Name)
		}
		q, _ := newSynth(t)
		if err := q.SetState(saved); err != nil {
			t.Fatal(err)
		}
		if got := q.sound(); got.Bank != s.Bank || got.Group != s.Group || got.Name != s.Name || !reflect.DeepEqual(got.Patch, s.Patch()) {
			t.Errorf("%s: loaded again, the state is %s/%s/%s, its patch the same %v", s.Name, got.Bank, got.Group, got.Name,
				reflect.DeepEqual(got.Patch, s.Patch()))
		}
	}
}

func TestTheStateIsPlainJSON(t *testing.T) {
	s, _ := Named("Juno pad")
	var v map[string]any
	if err := json.Unmarshal(s.State(), &v); err != nil {
		t.Fatal(err)
	}
	if v["Name"] != "Juno pad" || v["Bank"] != Library || v["Group"] != "Pads & strings" || v["Patch"] == nil {
		t.Errorf("the state reads %v", v)
	}
	if regexp.MustCompile(`":(0|""|false|null)[,}]`).Match(s.State()) {
		t.Errorf("the state keeps zero values: %s", s.State())
	}
}

func TestAnEmptyStatePlaysTheDefaultSound(t *testing.T) {
	p := synthOf(t, "Moog bass")
	if err := p.SetState(nil); err != nil {
		t.Fatal(err)
	}
	if p.sound().Name != DefaultSound || p.State() != nil {
		t.Errorf("an empty state plays %s, and saves as %q", p.sound().Name, p.State())
	}
}

func TestAStateItCannotReadChangesNothing(t *testing.T) {
	p := synthOf(t, "Moog bass")
	was := p.State()
	for _, bad := range []string{"{", `{"Name":"x"}`, `{"Name":"x","Patch":{"Kind":"organ"}}`, `{"Patch":{"Osc":[{"Wave":"wobble"}]}}`} {
		if err := p.SetState([]byte(bad)); err == nil {
			t.Errorf("state %s is taken", bad)
		}
		if !bytes.Equal(p.State(), was) || p.sound().Name != "Moog bass" {
			t.Errorf("state %s changes the sound to %s", bad, p.sound().Name)
		}
	}
}

func TestAPatchOfItsOwnPlays(t *testing.T) {
	p, _ := newSynth(t)
	own := State{Name: "Mine", Patch: &synth.Patch{Osc: []synth.Osc{{Wave: "square"}}, Amp: synth.Env{Sustain: 1, Release: 0.1}}}
	if err := p.SetState(own.Encode()); err != nil {
		t.Fatal(err)
	}
	out := plugtest.Play(t, p.Processor(), plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}, 24000, params(), note(1, 57, 0, 20000)...)
	if got := hz(out[0][4800:19200], 48000); math.Abs(got-220) > 2 {
		t.Errorf("a square patch of its own plays A3 at %.1f Hz", got)
	}
}

func TestTheSynthPlaysInTuneAtTheHostsRate(t *testing.T) {
	for _, rate := range []float64{48000, 44100, 96000} {
		p := synthOf(t, "Jupiter brass lead")
		setup := plug.Setup{Rate: rate, MaxBlock: 256, Channels: 2}
		n := int(rate)
		out := plugtest.Play(t, p.Processor(), setup, n, params(), note(1, 57, 0, n-1)...)
		// The lead's lowest saws play an octave down, at 110 Hz.
		if got := hz(out[0][n/2:], rate); math.Abs(got-110) > 2 {
			t.Errorf("at %.0f Hz, A3 on the lead sounds at %.1f Hz, not 110", rate, got)
		}
	}
}

func TestAPedalAndABendReachTheSynth(t *testing.T) {
	p := synthOf(t, "Jupiter brass lead")
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	events := append(note(1, 57, 0, 12000),
		plug.Event{Frame: 6000, Kind: plug.Pedal, Group: 0, Value: 1},
		plug.Event{Frame: 24000, Kind: plug.Expression, Note: 1, Expr: plug.Bend, Value: 12},
	)
	out := plugtest.Play(t, p.Processor(), setup, 48000, params(), events...)
	// The lead's lowest saws play an octave down, at 220 Hz bent.
	if got := hz(out[0][36000:], 48000); math.Abs(got-220) > 4 {
		t.Errorf("held by the pedal and bent up an octave, A3 on the lead sounds at %.1f Hz, not 220", got)
	}
}

func TestResetEndsTheNotes(t *testing.T) {
	p, _ := newSynth(t)
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	pr := p.Processor()
	plugtest.Play(t, pr, setup, 4800, params(),
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

// swapSounds are sounds a swap test steps through: a pad, a pluck, a
// mono bass, a drum kit, a General MIDI piano, and a song's patch.
var swapSounds = []string{"Juno pad", "Harp pluck", "Moog bass", "TR-808", "Acoustic Grand Piano", "Supersaw pad"}

// swapStates returns the states of swapSounds.
func swapStates(t testing.TB) [][]byte {
	t.Helper()
	var out [][]byte
	for _, n := range swapSounds {
		s, ok := Named(n)
		if !ok {
			t.Fatalf("no sound is named %s", n)
		}
		out = append(out, s.State())
	}
	if s, ok := Find(Songs, "Star Drift", "chime"); ok {
		out = append(out, s.State())
	} else {
		t.Fatal("Star Drift plays no chime")
	}
	return out
}

func TestProcessAllocatesNothingAsTheSoundChanges(t *testing.T) {
	p, _ := newSynth(t)
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	states := swapStates(t)
	l, r := make([]float32, 256), make([]float32, 256)
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 256, Events: make([]plug.Event, 4), Changes: make([]plug.Change, 1)}
	n := plugtest.AllocsUnder(t, "plugsynth.(*processor).Process", func() {
		for i := range 400 {
			if i%37 == 0 {
				// Made here, off the audio thread, as a host's app does.
				if err := p.SetState(states[i/37%len(states)]); err != nil {
					t.Fatal(err)
				}
			}
			id := int32(i)
			b.Events[0] = plug.Event{Frame: 10, Kind: plug.NoteOn, Note: id, Pitch: 48 + float64(i%24), Velocity: 0.8, Group: id % 2}
			b.Events[1] = plug.Event{Frame: 20, Kind: plug.Expression, Note: id, Expr: plug.Bend, Value: 0.5}
			b.Events[2] = plug.Event{Frame: 30, Kind: plug.Pedal, Group: id % 2, Value: float64(i % 2)}
			b.Events[3] = plug.Event{Frame: 200, Kind: plug.NoteOff, Note: id - 3}
			b.Changes[0] = plug.Change{Frame: 100, ID: CutoffID, Value: float64(i%9) - 4}
			pr.Process(b)
		}
	})
	if n != 0 {
		t.Errorf("400 blocks allocate %d times", n)
	}
}

func TestASoundChangedMidPlayKeepsTheNotesHeld(t *testing.T) {
	p := synthOf(t, "Juno pad")
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2})
	l, r := make([]float32, 128), make([]float32, 128)
	block := func(events ...plug.Event) float64 {
		b := &plug.Block{Out: [][]float32{l, r}, Frames: 128, Events: events}
		pr.Process(b)
		for i := range l {
			if math.IsNaN(float64(l[i])) || math.IsInf(float64(l[i]), 0) || math.Abs(float64(l[i])) > 4 {
				t.Fatalf("a frame is %v", l[i])
			}
		}
		return rms(l)
	}
	// A chord held, a note let go under the pedal, and a bend.
	block(plug.Event{Kind: plug.NoteOn, Note: 1, Pitch: 57, Velocity: 0.8},
		plug.Event{Kind: plug.NoteOn, Note: 2, Pitch: 60, Velocity: 0.8},
		plug.Event{Kind: plug.Pedal, Value: 1},
		plug.Event{Frame: 3, Kind: plug.NoteOn, Note: 3, Pitch: 64, Velocity: 0.8},
		plug.Event{Frame: 50, Kind: plug.NoteOff, Note: 3},
		plug.Event{Frame: 60, Kind: plug.Expression, Note: 2, Expr: plug.Bend, Value: 2})
	for range 150 {
		block()
	}
	states := swapStates(t)
	for i, st := range states {
		if err := p.SetState(st); err != nil {
			t.Fatal(err)
		}
		// Each frame of each block after the change, for a fifth of a
		// second.
		sum := 0.0
		for range 75 {
			sum += block()
		}
		// A drum kit strikes nothing, and keeps the notes for the next.
		if drums := i < len(swapSounds) && swapSounds[i] == "TR-808"; !drums && sum/75 < 0.003 {
			t.Errorf("after the change to sound %d, the notes held fall silent, at %.4f", i, sum/75)
		}
		if got := p.proc.cur.Held(); got != 3 {
			t.Errorf("after the change to sound %d, the sound playing holds %d notes, not 3", i, got)
		}
	}
	// Every note let go and the pedal lifted: within 12 s the sound is
	// silent, and so is the sound before, ringing out.
	block(plug.Event{Kind: plug.NoteOff, Note: 1}, plug.Event{Kind: plug.NoteOff, Note: 2}, plug.Event{Kind: plug.Pedal, Value: 0})
	for range 12 * 48000 / 128 {
		block()
	}
	if got := block(); got > 1e-5 || p.proc.cur.Sounding() || p.proc.cur.Held() != 0 {
		t.Errorf("with every note let go, a note sticks, at %.6f, the sound playing holding %d", got, p.proc.cur.Held())
	}
}

func TestTheSoundChangesSafelyWhileBlocksRun(t *testing.T) {
	p, _ := newSynth(t)
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	states := swapStates(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 40 {
			if err := p.SetState(states[i%len(states)]); err != nil {
				t.Error(err)
				return
			}
			_ = p.State()
		}
	}()
	l, r := make([]float32, 256), make([]float32, 256)
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 256, Events: make([]plug.Event, 2)}
	for i := 0; ; i++ {
		select {
		case <-done:
			return
		default:
		}
		b.Events[0] = plug.Event{Kind: plug.NoteOn, Note: int32(i), Pitch: 60 + float64(i%12), Velocity: 0.7}
		b.Events[1] = plug.Event{Frame: 128, Kind: plug.NoteOff, Note: int32(i - 2)}
		pr.Process(b)
		_ = pr.Tail()
	}
}

func TestTheViewsShowAndFollow(t *testing.T) {
	p, h := newSynth(t)
	play(t, p, "Moog bass")
	c, ok := p.View(plug.Chip, h).(*chip)
	if !ok || c.label.Text != "Moog bass" {
		t.Fatal("the chip shows another sound than the Moog bass")
	}
	play(t, p, "TR-808")
	w := gunimtest.New(t, geom.Sz(200, 30), nil)
	gunim.RegisterView(w, "chip", func(struct{}) gunim.Node { return c }, nil)
	if err := w.Client().Mount(gunim.Root, "chip", "chip", nil); err != nil {
		t.Fatal(err)
	}
	gunim.RegisterPatch(w, "chip", func(_ gunim.Node, _ struct{}, u *gunim.UI) { c.FollowState(u) })
	if err := w.Client().Patch("chip", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	if c.label.Text != "TR-808" {
		t.Errorf("the chip follows to %q, not TR-808", c.label.Text)
	}
	for _, size := range []plug.Size{plug.Panel, plug.Full} {
		pn, ok := p.View(size, h).(*panel)
		if !ok || pn.st.Name != "TR-808" {
			t.Fatalf("view %d shows another sound than the TR-808", size)
		}
		for i, r := range boundsOf(t, pn, geom.Sz(900, 420), pn.sections) {
			if r.Empty() || r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 900 || r.Max.Y > 420 {
				t.Errorf("view %d: section %s is at %v, outside the view", size, pn.sections[i].Name, r)
			}
		}
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
	p := synthOf(t, "TR-808")
	pr := p.Processor()
	setup := plug.Setup{Rate: 48000, MaxBlock: 128, Channels: 2}
	// Right after Prepare, at the first frame, and at frames inside and
	// at the start of later blocks.
	for _, f := range []int{0, 1, 77, 128, 300} {
		out := plugtest.Play(t, pr, setup, 1024, params(), drum(1, f))
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
		p := synthOf(t, "TR-808")
		pr := p.Processor()
		a := plugtest.Play(t, pr, setup, 4000, params(), drum(1, c.every))
		q := synthOf(t, "TR-808")
		b := plugtest.Play(t, q.Processor(), setup, 4000, params(), drum(1, 7*c.every))
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
	ps := params()
	ps[ReleaseID-CutoffID].Default = 8
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

func TestTheKnobsTurnTheSoundAndTheNextOne(t *testing.T) {
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	level := func(gain bool) float64 {
		p := synthOf(t, "Juno pad")
		pr := p.Processor()
		pr.Prepare(setup)
		l, r := make([]float32, 512), make([]float32, 512)
		var changes []plug.Change
		if gain {
			changes = []plug.Change{{ID: GainID, Value: -20}}
		}
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512, Changes: changes})
		// The sound changes after the gain is turned: it keeps the gain.
		play(t, p, "Jupiter brass lead")
		pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512,
			Events: []plug.Event{{Frame: 10, Kind: plug.NoteOn, Note: 1, Pitch: 57, Velocity: 1}}})
		sum := 0.0
		for range 40 {
			pr.Process(&plug.Block{Out: [][]float32{l, r}, Frames: 512})
			sum += rms(l)
		}
		return sum
	}
	if r := level(true) / level(false); math.Abs(r-0.1) > 0.01 {
		t.Errorf("-20 dB of gain turned before the sound changed turns the lead by %.3f, not 0.1", r)
	}
}

func TestPressureAndModLeaveThePitchBe(t *testing.T) {
	for _, e := range []plug.ExprKind{plug.Pressure, plug.Mod} {
		p := synthOf(t, "Jupiter brass lead")
		setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
		events := append(note(1, 57, 0, 47999), plug.Event{Frame: 100, Kind: plug.Expression, Note: 1, Expr: e, Value: 1})
		out := plugtest.Play(t, p.Processor(), setup, 48000, params(), events...)
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
	for _, c := range []struct {
		name             string
		filter, envelope bool
	}{
		{"Juno pad", true, true}, {"Moog bass", true, true}, {"DX7 E-piano", false, true}, {"TR-808", false, false},
	} {
		play(t, p, c.name)
		pn.st = p.sound()
		pn.dim()
		if knob(pn, CutoffID).Disabled == c.filter || knob(pn, ResID).Disabled == c.filter {
			t.Errorf("%s: the filter's knobs are grey %v, though it has a filter %v", c.name, knob(pn, CutoffID).Disabled, c.filter)
		}
		if knob(pn, ReleaseID).Disabled == c.envelope || knob(pn, AttackID).Disabled == c.envelope {
			t.Errorf("%s: the envelope's knobs are grey %v", c.name, knob(pn, ReleaseID).Disabled)
		}
		if knob(pn, GainID).Disabled {
			t.Errorf("%s: the gain is grey", c.name)
		}
	}
}

func TestTheSoundIsTheSameInAnyBlockSize(t *testing.T) {
	for _, name := range []string{"Juno pad", "Moog bass"} {
		var want []float32
		for _, block := range []int{256, 1, 37} {
			p := synthOf(t, name)
			setup := plug.Setup{Rate: 48000, MaxBlock: block, Channels: 2}
			events := append(note(1, 45, 10, 30000), note(2, 52, 9000, 20000)...)
			out := plugtest.Play(t, p.Processor(), setup, 40000, params(), events...)[0]
			if want == nil {
				want = out
				continue
			}
			for i := range want {
				if out[i] != want[i] {
					t.Errorf("%s in blocks of %d differs from blocks of 256 from frame %d", name, block, i)
					break
				}
			}
		}
	}
}

func TestLatencyAndTailReadWhileBlocksRun(t *testing.T) {
	p := synthOf(t, "Moog bass")
	pr := p.Processor()
	pr.Prepare(plug.Setup{Rate: 44100, MaxBlock: 256, Channels: 2})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				if pr.Tail() < 0 || pr.Latency() != 2 {
					t.Error("Tail or Latency read wrong while blocks run")
					return
				}
			}
		}
	}()
	l, r := make([]float32, 256), make([]float32, 256)
	b := &plug.Block{Out: [][]float32{l, r}, Frames: 256, Changes: make([]plug.Change, 1)}
	for i := range 291 {
		b.Changes[0] = plug.Change{Frame: 100, ID: ReleaseID, Value: 0.125 * float64(1+i%64)}
		pr.Process(b)
	}
	close(stop)
	<-done
	// The last block set the release ×4.375.
	want := int(math.Ceil(preset(t, "Moog bass").Amp.Release*4.375*44100)) + 2
	if got := pr.Tail(); got != want {
		t.Errorf("the tail is %d frames, want %d", got, want)
	}
}

// preset returns the patch library's preset named name.
func preset(t *testing.T, name string) *synth.Patch {
	t.Helper()
	ps, err := music.Presets()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Name == name {
			return p.Patch
		}
	}
	t.Fatalf("the library has no preset %s", name)
	return nil
}
