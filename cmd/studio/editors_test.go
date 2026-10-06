package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/input"

	music "github.com/marrasen/gunim-music"
	"github.com/marrasen/gunim-music/synth"
)

func TestPathsReadAndSetTheSong(t *testing.T) {
	s, _ := music.Song(music.KeypadRound)
	ss, ok := s.(*synth.Song)
	if !ok {
		t.Fatal("Keypad Round is made in code")
	}
	song := ss.Clone()
	for path, want := range map[string]float64{
		"Patches/lead/Osc/1/Detune": 6,
		"Tracks/pad/Gain":           -11,
		"Mix/Reverb/Size":           0.95,
		"BPM":                       118,
		"Patches/kit/Kit/cp/Gain":   0.75,
	} {
		if got := num(song, path); got != want {
			t.Errorf("%s is %v, want %v", path, got, want)
		}
	}
	if str(song, "Patches/lead/Osc/0/Wave") != "pulse" {
		t.Errorf("the lead's first wave is %q", str(song, "Patches/lead/Osc/0/Wave"))
	}
	for path, x := range map[string]any{
		"Patches/lead/Filter/Cutoff": 800.0,
		"Tracks/bass/Mute":           1.0,
		"Patches/kit/Kit/zap/Tune":   3.0,
		"Patches/pad/Vowel":          "o",
		"Mix/Transitions/Lift":       2.0,
		"Patches/bass/Poly":          3.4,
	} {
		if err := setPath(song, path, x); err != nil {
			t.Errorf("setting %s: %v", path, err)
		}
	}
	if song.Patches["lead"].Filter.Cutoff != 800 || !track(song, "bass").Mute || song.Patches["kit"].Kit["zap"].Tune != 3 ||
		song.Patches["pad"].Vowel != "o" || song.Mix.Transitions.Lift != 2 || song.Patches["bass"].Poly != 3 {
		t.Error("a value set did not take")
	}
	if err := setPath(song, "Patches/lead/Nothing", 1.0); err == nil {
		t.Error("a path to nothing was set")
	}
	if err := addItem(song, "Patches/lead/Osc"); err != nil || len(song.Patches["lead"].Osc) != 4 || song.Patches["lead"].Osc[3].Wave != "saw" {
		t.Errorf("adding an oscillator: %v, %d oscillators", err, len(song.Patches["lead"].Osc))
	}
	if err := removeItem(song, "Patches/lead/Osc", 0); err != nil || song.Patches["lead"].Osc[0].Wave != "saw" {
		t.Errorf("taking the first oscillator out: %v", err)
	}
}

func TestPatternOps(t *testing.T) {
	for _, c := range []struct {
		src  string
		op   PatternOp
		want string
		at   string
	}{
		{"bd ~ sn ~", PatternOp{At: "1", Op: "set", Arg: "hh"}, "bd hh sn ~", "1"},
		{"bd ~ sn ~", PatternOp{At: "0", Op: "split", Arg: "2"}, "[bd bd] ~ sn ~", "0/0"},
		{"bd ~ cp ~", PatternOp{At: "2", Op: "after"}, "bd ~ cp" + " cp ~", "3"},
		{"bd ~ sn ~", PatternOp{At: "3", Op: "delete"}, "bd ~ sn", "2"},
		{"bd ~ sn ~", PatternOp{At: "0", Op: "alt"}, "<bd bd> ~ sn ~", "0/1"},
		{"bd ~ sn ~", PatternOp{At: "0", Op: "fast", Arg: "1"}, "bd*2 ~ sn ~", "0"},
		{"bd*2 ~ sn ~", PatternOp{At: "0", Op: "fast", Arg: "-1"}, "bd ~ sn ~", "0"},
		{"bd ~ sn ~", PatternOp{At: "0", Op: "fast", Arg: "-1"}, "bd/2 ~ sn ~", "0"},
		{"bd ~ sn ~", PatternOp{At: "0", Op: "euclid"}, "bd(3,8) ~ sn ~", "0"},
		{"bd(3,8) ~", PatternOp{At: "0", Op: "euclid", Arg: "1"}, "bd(4,8) ~", "0"},
		{"bd(3,8) ~", PatternOp{At: "0", Op: "steps", Arg: "-1"}, "bd(3,7) ~", "0"},
		{"hh*8", PatternOp{At: "", Op: "maybe"}, "hh*8?", ""},
		{"hh*8?", PatternOp{At: "", Op: "maybe"}, "hh*8", ""},
		{"0 2", PatternOp{At: "1", Op: "weight", Arg: "1"}, "0 2@2", "1"},
		{"0 2", PatternOp{At: "0", Op: "layer"}, "[0, 0] 2", "0/1"},
		{"bd", PatternOp{At: "", Op: "delete"}, "~", ""},
		{"bd ~", PatternOp{At: "1", Op: "set", Arg: "[cp cp]"}, "bd [cp cp]", "1"},
	} {
		got, at, err := applyOp(c.src, c.op)
		if err != nil || got != c.want || at != c.at {
			t.Errorf("%q %s %s at %q: got %q at %q, %v; want %q at %q", c.src, c.op.Op, c.op.Arg, c.op.At, got, at, err, c.want, c.at)
		}
	}
	if _, _, err := applyOp("bd sn", PatternOp{At: "5", Op: "set", Arg: "x"}); err == nil {
		t.Error("a step that is not there was changed")
	}
}

func TestTheGridWritesBackWhatItReads(t *testing.T) {
	for _, c := range []struct {
		src  string
		cols int
	}{
		{"bd ~ sn ~", 16},
		{"[bd*4, ~ cp ~ cp, hh*8]", 16},
		{"0 _ 2 4", 16},
		{"<[bd ~ ~ bd] [bd bd ~ ~]>", 16},
		{"[c0 c2 c1 c2]*2", 16},
	} {
		n := synth.PatternCycles(c.src)
		grids := make([]gridOf, n)
		for k := range n {
			grids[k] = readGrid(c.src, k, c.cols, nil)
		}
		back := writeGrid(grids)
		if !slices.Equal(evsOf(back, n), evsOf(c.src, n)) {
			t.Errorf("%q came back from the grid as %q, which plays otherwise", c.src, back)
		}
	}
	if got := writeRow("hh", []int8{1, 0, 1, 0, 1, 0, 1, 0}); got != "[hh ~]*4" {
		t.Errorf("a row of hats every other step writes as %q", got)
	}
}

func TestEditorsEditTheSongAsItPlays(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.do(SetValue{Path: "Patches/lead/Filter/Cutoff", Num: 900})
	if got := h.s.song.Patches["lead"].Filter.Cutoff; got != 900 {
		t.Errorf("the lead's cutoff is %v", got)
	}
	h.do(SetValue{Path: "Patches/lead/Osc/0/Wave", Str: "zap", IsStr: true})
	if h.s.song.Patches["lead"].Osc[0].Wave != "pulse" || !strings.Contains(h.s.status, "zap") {
		t.Errorf("a wave of zap was taken, or not said why: %q", h.s.status)
	}
	h.do(AddItem{Path: "Patches/lead/LFO"})
	if n := len(h.s.song.Patches["lead"].LFO); n != 2 {
		t.Errorf("the lead has %d LFOs after one was added, want 2", n)
	}
	h.do(PatchNew{Kind: "synth"})
	if _, ok := h.s.song.Patches["synth 2"]; !ok {
		t.Errorf("no new patch among %v", sortedNames(h.s.song.Patches))
	}
	h.do(PatchCopy{Name: "lead"})
	if _, ok := h.s.song.Patches["lead 2"]; !ok {
		t.Errorf("no copy of the lead among %v", sortedNames(h.s.song.Patches))
	}
	h.do(TrackPatch{Track: "arp", Patch: "lead 2"})
	if tr := track(h.s.song, "arp"); tr.Patch != "lead 2" || tr.Pattern != "x*8" {
		t.Errorf("the arp plays %s, %q", tr.Patch, tr.Pattern)
	}
	h.do(TrackPatch{Track: "arp", Patch: "kit"})
	if tr := track(h.s.song, "arp"); tr.Patch != "kit" || tr.Pattern != "bd ~ sn ~" {
		t.Errorf("the arp, given a kit, plays %s, %q", tr.Patch, tr.Pattern)
	}
	h.do(PatternOp{Track: "bass", At: "0", Op: "split", Arg: "2"})
	if got := track(h.s.song, "bass").Pattern; !strings.HasPrefix(got, "[b b]") {
		t.Errorf("the bass's pattern became %q", got)
	}
	if st := h.s.state(); st.PatternSel != "0/0" || st.PatternTrack != "bass" {
		t.Errorf("the editor chooses %q of %q after the change", st.PatternSel, st.PatternTrack)
	}
	h.do(SetValue{Path: "Tracks/pad/Solo", Num: 1})
	h.play(time.Second)
	for _, r := range h.s.state().Tracks {
		if r.Name != "pad" && r.Meter.Peak[0] > 0.01 {
			t.Errorf("%s is heard at %v while the pad is soloed", r.Name, r.Meter.Peak[0])
		}
	}
}

func TestEditorsRenderWhatTheyShow(t *testing.T) {
	h := newHarness(t, music.KeypadRound)
	h.do(Focus{Patch: "lead", Kit: "kit", Drum: "sn", Track: "bass"}, Audition{Patch: "lead", Pitch: 72}, Audition{Patch: "kit", Drum: "cp"})
	h.play(500 * time.Millisecond)
	st := h.s.state()
	if st.Preview.Patch != "lead" || len(st.Preview.Wave) != 720 || len(st.Preview.Cycle) != 256 {
		t.Errorf("the lead's preview is %q, %d, %d", st.Preview.Patch, len(st.Preview.Wave), len(st.Preview.Cycle))
	}
	if st.DrumPreview.Drum != "sn" || len(st.DrumPreview.Wave) == 0 {
		t.Errorf("the snare's preview is %+v", st.DrumPreview.Drum)
	}
	loud := false
	for _, v := range st.Scope {
		loud = loud || v > 0.01 || v < -0.01
	}
	if st.ScopeTrack != "bass" || !loud {
		t.Errorf("the scope shows %q, loud %v", st.ScopeTrack, loud)
	}
	if st.Master.Peak[0] <= 0 || st.LUFS < -70 {
		t.Errorf("the master shows %v and %.1f LUFS", st.Master, st.LUFS)
	}
	// Each editor shows, and a knob turned sets its value.
	for _, e := range editorTabs {
		h.do(OpenEditor{Editor: e})
	}
	k := h.v.patch.fKnobs[0]
	if k.path() != "Patches/"+h.v.patch.name+"/Filter/Cutoff" {
		t.Fatalf("the cutoff knob sets %s", k.path())
	}
	h.do(OpenEditor{Editor: "Patch"})
	h.settle()
	at, ok := h.boundsOf(k)
	if !ok {
		t.Fatal("the cutoff knob is not on screen")
	}
	c := geomPt((at.Min.X+at.Max.X)/2, (at.Min.Y+at.Max.Y)/2)
	h.w.Input(input.PointerDown{Pos: c, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerMove{Pos: c.Add(geomPt(0, -40)), Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: c.Add(geomPt(0, -40)), Time: time.Now()})
	got, ok := h.intent().(SetValue)
	if !ok || got.Path != k.path() || got.Num <= num(h.s.song, k.path()) {
		t.Errorf("a drag up the cutoff knob sent %#v", got)
	}
}
