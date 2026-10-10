package main

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

// harness runs the studio's two halves in a test: the window offscreen,
// and the sound mixed by hand, with no speaker.
type harness struct {
	t *testing.T
	w *gunim.Window
	s *studio
	v *root
	// stageAt is where the stage lay at the last update, and measure
	// where the nodes asked about lay.
	stageAt geom.Rect
	measure map[gunim.Node]geom.Rect
}

// boundsOf returns where n lies in the window.
func (h *harness) boundsOf(n gunim.Node) (geom.Rect, bool) {
	if h.measure == nil {
		h.measure = map[gunim.Node]geom.Rect{}
	}
	h.measure[n] = geom.Rect{}
	h.frame()
	h.frame()
	r := h.measure[n]
	return r, !r.Empty()
}

func geomPt(x, y float32) geom.Point { return geom.Pt(x, y) }

func newHarness(t *testing.T, song string) *harness {
	t.Helper()
	h := &harness{t: t, w: gunimtest.New(t, geom.Sz(1500, 940), widget.NewSurface())}
	h.w.RegisterTheme(widget.Dark())
	gunim.RegisterView(h.w, "studio", func(s Studio) *root {
		h.v = buildView(s)
		return h.v
	}, func(r *root, s Studio, u *gunim.UI) {
		r.update(s, u)
		h.stageAt, _ = u.Bounds(r.stage)
		for n := range h.measure {
			h.measure[n], _ = u.Bounds(n)
		}
	})
	s, err := newStudio(h.w.Client(), audio.NewMixer(), song)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	if err := h.w.Client().Mount(gunim.Root, "studio", "studio", s.state()); err != nil {
		t.Fatal(err)
	}
	h.frame()
	return h
}

// play mixes d of sound, and shows the studio as it is then.
func (h *harness) play(d time.Duration) {
	buf := make([]float32, 2*1024)
	for n := audio.Frames(d); n > 0; n -= 1024 {
		h.s.mix.Mix(buf)
	}
	h.frame()
}

// do carries out intents, as the window sends them, and shows the
// studio as it is then.
func (h *harness) do(vs ...gunim.Intent) {
	for _, v := range vs {
		h.s.handle(h.t.Context(), v)
	}
	h.frame()
}

func (h *harness) frame() {
	_ = h.w.Client().Update("studio", h.s.state())
	h.w.Frame(time.Second / 60)
	h.w.Frame(time.Second / 60)
}

// settle runs frames for a second, for what moves to come to rest, as a
// tab sliding in does.
func (h *harness) settle() {
	_ = h.w.Client().Update("studio", h.s.state())
	for range 60 {
		h.w.Frame(time.Second / 60)
	}
}

// intent waits for the window to send an intent, passing by those that
// say what the editors show, which it sends as they change.
func (h *harness) intent() gunim.Intent {
	for {
		select {
		case ev := <-h.w.Client().Intents():
			if _, ok := ev.Intent.(Focus); ok {
				continue
			}
			return ev.Intent
		case <-time.After(time.Second):
			h.t.Fatal("the window sent no intent")
			return nil
		}
	}
}

func (h *harness) row(name string) TrackRow {
	for _, r := range h.s.state().Tracks {
		if r.Name == name {
			return r
		}
	}
	h.t.Fatalf("no track %s", name)
	return TrackRow{}
}

func TestAPatternTypedIsHeardFromTheNextBar(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.play(time.Second)
	h.do(PatternEdited{Track: "bass", Text: "c0*8"})
	if r := h.row("bass"); r.Pattern != "c0*8" || r.PatternErr != "" {
		t.Fatalf("the bass's card shows %q, %q", r.Pattern, r.PatternErr)
	}
	bar := time.Second * 60 * 4 / 118
	h.play(2 * bar)
	from := h.s.heard() - int64(h.s.state().BarFrames)
	n := 0
	for _, note := range h.s.p.Notes(nil, from) {
		if note.Track == "bass" && note.Frame >= from {
			n++
		}
	}
	if n < 8 {
		t.Errorf("the bass played %d notes in the last bar, want 8", n)
	}
}

func TestABrokenPatternSaysWhyAndPlaysOn(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.do(PatternEdited{Track: "bass", Text: "[c0 c2"})
	r := h.row("bass")
	if r.Pattern != "[c0 c2" || !strings.Contains(r.PatternErr, "[ needs its ]") {
		t.Errorf("the bass's card shows %q, %q", r.Pattern, r.PatternErr)
	}
	if got := track(h.s.song, "bass").Pattern; got != "b _ _ ~ b _ ~ c2" {
		t.Errorf("the song took the broken pattern: %q", got)
	}
	h.do(PatternEdited{Track: "bass", Text: "[c0 c2]"})
	if r := h.row("bass"); r.PatternErr != "" {
		t.Errorf("the fixed pattern still shows %q", r.PatternErr)
	}
}

func TestChordsTypedAndWritten(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	gen := h.s.gen
	h.do(ChordsEdited{Text: "Am, F, C, G"})
	if got := h.s.song.Chords; !slices.Equal(got, []string{"Am", "F", "C", "G"}) {
		t.Errorf("the song's chords are %v", got)
	}
	h.do(ChordsEdited{Text: "Am Hm"})
	if st := h.s.state(); st.ChordErr == "" || !slices.Equal(h.s.song.Chords, []string{"Am", "F", "C", "G"}) {
		t.Errorf("after Hm, the error is %q and the chords %v", st.ChordErr, h.s.song.Chords)
	}
	h.do(ChordsGenerated{Style: "kpop"})
	st := h.s.state()
	if st.ChordErr != "" || st.Gen == gen || len(h.s.song.Chords) != 4 || st.ChordText != strings.Join(h.s.song.Chords, " ") {
		t.Errorf("writing chords left %+v and the chords %v", st.ChordErr, h.s.song.Chords)
	}
	if got := h.v.chords.Text(); got != st.ChordText {
		t.Errorf("the chords' field shows %q, want %q", got, st.ChordText)
	}
}

func TestTransposeMovesTheKeyAndTheChords(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.do(Transposed{By: 2})
	if st := h.s.state(); st.Key != "D major" || h.s.song.Chords[1] != "Bm7" {
		t.Errorf("transposed to %s, chords %v", st.Key, h.s.song.Chords)
	}
}

func TestTheFilterSliderSetsThePatchCutoff(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.do(KnobSet{Track: "pad", Knob: "filter", Value: 0.5})
	if got := h.s.song.Patches["pad"].Filter.Cutoff; math.Abs(got-640) > 1 {
		t.Errorf("the pad's cutoff is %.0f Hz, want 640", got)
	}
	h.do(KnobSet{Track: "claps", Knob: "filter", Value: 0.5})
	if got := track(h.s.song, "claps").LPF; math.Abs(got-640) > 1 {
		t.Errorf("the claps' lowpass is %.0f Hz, want 640", got)
	}
	h.do(KnobSet{Track: "pad", Knob: "gain", Value: -20})
	if r := h.row("pad"); r.Gain != -20 {
		t.Errorf("the pad's level is %v", r.Gain)
	}
}

func TestATapOnAnOrbStartsItsTrack(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.play(500 * time.Millisecond)
	st := h.v.stage
	i := slices.IndexFunc(st.s.Tracks, func(r TrackRow) bool { return r.Name == "lead" })
	v := st.view()
	sc := st.scene(float64(h.s.heard()))
	m := sc.Camera.Matrix(v.Size().W / v.Size().H)
	p := m.Apply(st.orbAt(i))
	tap := geom.Pt(v.Min.X+(p.X+1)/2*v.Size().W, v.Min.Y+(1-p.Y)/2*v.Size().H).Add(h.stageAt.Min)
	now := time.Now()
	h.w.Input(input.PointerDown{Pos: tap, Time: now})
	h.w.Input(input.PointerUp{Pos: tap, Time: now})
	got, ok := h.intent().(PartClicked)
	if !ok || got.Name != "lead" {
		t.Fatalf("a tap on the lead's orb sent %#v", got)
	}
	h.do(got)
	if c := h.s.p.Part("lead"); c != 1 {
		t.Errorf("the lead's control is %v, want on", c)
	}
}

func TestTheStingPlaysAndEndsTheSong(t *testing.T) {
	h := newHarness(t, music.BossEntrance)
	h.play(time.Second)
	if st := h.s.state(); st.Tiers != 4 || !slices.Equal(st.Stings, []string{"victory"}) || h.v.sting.Disabled {
		t.Fatalf("the boss shows %d tiers and stings %v", st.Tiers, st.Stings)
	}
	h.do(StingPlayed{Name: "victory"})
	h.play(time.Second)
	if st := h.s.state(); st.Sting != "victory" || !strings.Contains(st.Status, "victory") {
		t.Errorf("after the sting, it shows %q, %q", st.Sting, st.Status)
	}
	h.do(Restarted{})
	h.play(time.Second)
	if st := h.s.state(); st.Sting != "" || st.Stopped {
		t.Errorf("after Restart, the sting is %q and stopped is %v", st.Sting, st.Stopped)
	}
}

func TestTheDigitKeysPlayTheKeypad(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.w.Input(input.KeyPress{Key: input.Key1, Char: '1', Time: time.Now()})
	if got, ok := h.intent().(KeyPlayed); !ok || got.Digit != 1 {
		t.Fatalf("the key 1 sent %#v", got)
	}
	h.w.Input(input.KeyPress{Key: input.KeyF1 + 2, Time: time.Now()})
	if got, ok := h.intent().(TierChosen); !ok || got.Tier != 3 {
		t.Fatalf("F3 sent %#v", got)
	}
}

func TestEverySongOpensAndPlays(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	for i := range h.s.songs {
		h.do(SongChosen{Song: i})
		h.play(2 * time.Second)
		st := h.s.state()
		if st.Song != i || len(st.Tracks) == 0 || len(st.Notes) == 0 || len(st.Spans) == 0 {
			t.Errorf("song %d shows %d tracks, %d notes, %d chords", i, len(st.Tracks), len(st.Notes), len(st.Spans))
		}
	}
}

func TestSpansLayTheChordsOutAboutTheFrameHeard(t *testing.T) {
	look := synth.Look{Bar: 5, BarFrame: 5000, BarFrames: 1000, Chords: []string{"C", "Am", "F", "G"}, ChordIndex: 1}
	got := spans(look, &synth.Song{}, 5500)
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Name)
	}
	// From bar 2 to bar 7, Am at bar 5, heard.
	if !slices.Equal(names, []string{"F", "G", "C", "Am", "F", "G"}) || got[3].Frame != 5000 || got[3].Len != 1000 {
		t.Errorf("laid out %v, the heard one %+v", names, got[3])
	}
}

func TestFilterSliderRunsByOctaves(t *testing.T) {
	for _, x := range []float32{0, 0.25, 0.5, 1} {
		if got := filterAt(cutoffAt(x)); math.Abs(float64(got-x)) > 1e-5 {
			t.Errorf("%v went to %v Hz and back to %v", x, cutoffAt(x), got)
		}
	}
}

func TestAWanderSongShowsItsOneLabelChosen(t *testing.T) {
	// From a tiers song at tier 3 to a wander song, the tiers' highlight
	// goes to the one label, "wanders", not where tier 3's button was.
	h := newHarness(t, music.KeypadRound)
	h.do(TierChosen{Tier: 3})
	h.settle()
	h.do(SongChosen{Song: slices.Index(h.s.state().Songs, "Compass Rose")})
	h.settle()
	if got := h.v.tiers.Selected(); got != 0 || !slices.Equal(h.v.tiers.Items, []string{"wanders"}) {
		t.Errorf("Compass Rose's tiers show %v, %d chosen", h.v.tiers.Items, got)
	}
}

func TestPadsAndKeysPlayWhileTheSongIsPaused(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.play(time.Second)
	h.do(PlayToggled{})
	if h.s.state().Playing {
		t.Fatal("the song plays on after Pause")
	}
	// Paused, the mix falls silent.
	buf := make([]float32, 2*1024)
	loud := func(blocks int) float32 {
		var top float32
		for range blocks {
			h.s.mix.Mix(buf)
			for _, v := range buf {
				top = max(top, abs(v))
			}
		}
		return top
	}
	loud(60)
	if q := loud(10); q > 0.01 {
		t.Fatalf("paused, the mix still peaks at %.3f", q)
	}
	for _, try := range []gunim.Intent{
		Audition{Patch: "kit", Drum: "sn"},
		Audition{Patch: "lead", Pitch: 72},
		KeyPlayed{Digit: 5},
	} {
		loud(30)
		h.do(try)
		if q := loud(10); q < 0.02 {
			t.Errorf("paused, %#v sounds at %.4f", try, q)
		}
	}
	// And the song stays where it was paused.
	if h.s.state().Playing {
		t.Error("trying a pad started the song")
	}
}

func TestAQuickSecondClickUndoesTheFirst(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	// Two clicks before the window shows the first: each turns over the
	// value as it is when it arrives, not as the window last showed it.
	for _, v := range []gunim.Intent{
		ToggleValue{Path: "Tracks/bass/Solo"},
		ToggleValue{Path: "Patches/lead/Vocoder"},
		ToggleValue{Path: "Mix/Gated", Seed: "Mix/Gated/Hold", Num: 0.3},
		ToggleValue{Path: "Patches/lead/Chip", Seed: "Patches/lead/Chip/Levels", Num: 16},
		ArpSet{Patch: "lead"},
	} {
		h.s.handle(t.Context(), v)
		h.s.handle(t.Context(), v)
	}
	h.frame()
	s := h.s.song
	if track(s, "bass").Solo || s.Patches["lead"].Vocoder || s.Mix.Gated != nil || s.Patches["lead"].Chip != nil ||
		s.Patches["lead"].Arpeggio != nil {
		t.Error("a second click quick after the first left its value changed")
	}
	h.do(ToggleValue{Path: "Tracks/bass/Solo"}, ToggleValue{Path: "Mix/Gated", Seed: "Mix/Gated/Hold", Num: 0.3})
	if !track(h.s.song, "bass").Solo || h.s.song.Mix.Gated == nil || h.s.song.Mix.Gated.Hold != 0.3 {
		t.Error("one click turned nothing on")
	}
}

func TestPickingAChorusKindIsHeard(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	b := track(h.s.song, "bass")
	b.Chorus = 0
	h.do(ChorusKind{Track: "bass", Type: "juno1"})
	if b := track(h.s.song, "bass"); b.ChorusType != "juno1" || b.Chorus != 0.5 {
		t.Errorf("the bass's chorus is %q at %.2f; want juno1, turned up", b.ChorusType, b.Chorus)
	}
	track(h.s.song, "bass").Chorus = 0.2
	h.do(ChorusKind{Track: "bass", Type: "ensemble"})
	if b := track(h.s.song, "bass"); b.ChorusType != "ensemble" || b.Chorus != 0.2 {
		t.Errorf("the bass's chorus is %q at %.2f; want ensemble, at 0.2 as it was", b.ChorusType, b.Chorus)
	}
}

func TestTheChorusSwitchPicksAKind(t *testing.T) {
	h := newHarness(t, music.NotteDiNeon)
	h.do(OpenEditor{Editor: "Mixer"})
	h.settle()
	var sw *selector
	for i, r := range h.s.state().Tracks {
		if r.Name == "hook" {
			sw = h.v.mixer.strips[i].chorus.sw
		}
	}
	if sw == nil || sw.sel != 2 || sw.off {
		t.Fatalf("the hook's chorus switch shows %+v; want Juno II, on", sw)
	}
	at, ok := h.boundsOf(sw)
	if !ok {
		t.Fatal("the hook's chorus switch is not on screen")
	}
	// The fifth lamp, the ensemble's.
	c := geomPt(at.Max.X-(at.Max.X-at.Min.X)/10, at.Min.Y+6)
	h.w.Input(input.PointerDown{Pos: c, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: c, Button: input.ButtonPrimary, Time: time.Now()})
	got, ok := h.intent().(ChorusKind)
	if !ok || got != (ChorusKind{Track: "hook", Type: "ensemble"}) {
		t.Errorf("a click on the last lamp sent %#v", got)
	}
}

func TestAPresetTriedIsAsLoudAndRevertsAndKeeps(t *testing.T) {
	h := newHarness(t, music.RingMeTwice)
	st := h.s.state()
	if len(st.Presets) < 40 {
		t.Fatalf("the studio offers %d presets", len(st.Presets))
	}
	orig := clonePatch(h.s.song.Patches["bass"])
	h.do(PresetTry{Patch: "bass", ID: "Bass/Bright Juno bass"})
	got := h.s.song.Patches["bass"]
	if got.Filter.Cutoff != 520 || h.s.state().Trying["bass"] != "Bass/Bright Juno bass" {
		t.Fatalf("the bass is not the preset tried: %+v", got.Filter)
	}
	// As loud as the bass was, at the octave its track plays in.
	pitch := h.s.pitchOf("bass")
	if a, b := levelOf(orig, pitch), levelOf(got, pitch); math.Abs(20*math.Log10(b/a)) > 0.5 {
		t.Errorf("the preset plays %.2f dB from the bass it replaced", 20*math.Log10(b/a))
	}
	// On through the category, then back.
	h.do(PresetStep{Patch: "bass", By: 1})
	if id := h.s.state().Trying["bass"]; id != "Bass/Hi-NRG roller" {
		t.Errorf("the next preset is %q", id)
	}
	h.do(PresetStep{Patch: "bass", By: -2})
	if id := h.s.state().Trying["bass"]; id != "Bass/Juno octave bass" {
		t.Errorf("two back is %q", id)
	}
	h.do(PresetStep{Patch: "bass"})
	if id := h.s.state().Trying["bass"]; !strings.HasPrefix(id, "Bass/") || id == "Bass/Juno octave bass" {
		t.Errorf("a preset at random is %q", id)
	}
	// A kit takes no synth's place.
	h.do(PresetTry{Patch: "bass", ID: "Drum kits/TR-808"})
	if h.s.song.Patches["bass"].Kind == "drums" {
		t.Error("a kit took the bass's place")
	}
	h.do(PresetRevert{Patch: "bass"})
	if a, b := jsonOf(h.s.song.Patches["bass"]), jsonOf(orig); a != b {
		t.Errorf("reverted, the bass is\n%s\nnot\n%s", a, b)
	}
	if _, ok := h.s.state().Trying["bass"]; ok {
		t.Error("the bass is still tried after Revert")
	}
	// A song's own patch, kept.
	h.do(PresetTry{Patch: "kit", ID: "From the songs/Notte di Neon/kit"}, PresetKeep{Patch: "kit"})
	if _, ok := h.s.state().Trying["kit"]; ok || h.s.song.Patches["kit"].Kit["syn1"].Pan != -0.45 {
		t.Error("the kit kept is not Notte di Neon's, or is still tried")
	}
}

func jsonOf(p *synth.Patch) string {
	b, _ := json.Marshal(p)
	return string(b)
}

func TestTheTweaksSetAndReset(t *testing.T) {
	h := newHarness(t, music.NotteDiNeon)
	h.do(SetValue{Path: "Mix/Tweak/Tone", Num: 0.6}, SetValue{Path: "Mix/Tweak/LoFi", Num: 0.4})
	if tw := h.s.song.Mix.Tweak; tw.Tone != 0.6 || tw.LoFi != 0.4 {
		t.Fatalf("the tweaks are %+v", tw)
	}
	h.do(ClearValue{Path: "Mix/Tweak"})
	if tw := h.s.song.Mix.Tweak; tw != (synth.Tweak{}) {
		t.Errorf("reset, the tweaks are %+v", tw)
	}
}

func TestThePatchPageStepsThroughTheTracks(t *testing.T) {
	h := newHarness(t, music.NotteDiNeon)
	h.do(OpenEditor{Editor: "Patch", Track: "bass"})
	h.settle()
	pp := h.v.patch
	if pp.name != "bass" || pp.track != "bass" || pp.stripSw.which != 0 {
		t.Fatalf("the patch page shows %s, played by %s, its strip %d", pp.name, pp.track, pp.stripSw.which)
	}
	// On past the drums, which the kit editor edits, to the next synth.
	want := []string{"arp", "hook", "voice", "choir", "strings", "epiano", "orch", "zaps", "bass"}
	for _, w := range want {
		f, _ := pp.stepTrack(1).(Focus)
		h.frame()
		if pp.track != w || pp.name != track(h.s.song, w).Patch || f.Track != w {
			t.Fatalf("a step on shows the track %s and the patch %s, the scope on %q; want %s", pp.track, pp.name, f.Track, w)
		}
	}
	pp.stepTrack(-1)
	h.frame()
	if pp.track != "zaps" {
		t.Errorf("a step back shows %s, want zaps", pp.track)
	}
}

func TestAWaveLampSetsTheWave(t *testing.T) {
	h := newHarness(t, music.NotteDiNeon)
	h.do(OpenEditor{Editor: "Patch", Track: "bass"})
	h.settle()
	sel := h.v.patch.oscs[0].wave
	if waves[sel.Selected()] != "saw" {
		t.Fatalf("the bass's first wave shows %s", waves[sel.Selected()])
	}
	at, ok := h.boundsOf(sel)
	if !ok {
		t.Fatal("the wave lamps are not on screen")
	}
	// The second lamp, the pulse's.
	c := sel.cells[1]
	p := geomPt(at.Min.X+(c.Min.X+c.Max.X)/2, at.Min.Y+(c.Min.Y+c.Max.Y)/2)
	h.w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Time: time.Now()})
	got, ok := h.intent().(SetValue)
	if !ok || got.Path != "Patches/bass/Osc/0/Wave" || got.Str != "pulse" {
		t.Errorf("a click on the pulse's lamp sent %#v", got)
	}
}
