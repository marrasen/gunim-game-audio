package synth_test

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"

	music "github.com/marrasen/gunim-music"
	"github.com/marrasen/gunim-music/synth"
)

// songs returns the library's songs made in code, by name.
func songs(t testing.TB) map[string]*synth.Song {
	t.Helper()
	out := map[string]*synth.Song{}
	for _, name := range music.Songs() {
		s, err := music.Song(name)
		if err != nil {
			t.Fatal(err)
		}
		if ss, ok := s.(*synth.Song); ok {
			out[name] = ss
		}
	}
	if len(out) < 3 {
		t.Fatalf("the library has %d songs made in code, want 3 or more", len(out))
	}
	return out
}

// play reads frames from p and returns them.
func play(p audio.Source, frames int) []float32 {
	out := make([]float32, 2*frames)
	for done := 0; done < frames; {
		n := min(frames-done, 1000)
		got, _ := p.Read(out[2*done : 2*(done+n)])
		done += got
	}
	return out
}

func stats(x []float32) (rms, peak, mean float64) {
	for _, s := range x {
		v := float64(s)
		rms += v * v
		peak = max(peak, math.Abs(v))
		mean += v
	}
	return math.Sqrt(rms / float64(len(x))), peak, mean / float64(len(x))
}

func phrase(p *synth.Player) int {
	l := p.Look(0)
	return int(float64(l.PhraseBars) * l.BarFrames)
}

func TestEverySongPlaysCleanly(t *testing.T) {
	for name, s := range songs(t) {
		p := synth.NewPlayer(s, 1)
		if err := p.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		p.SetTier(p.Tiers())
		x := play(p, 2*phrase(p))
		for i, v := range x {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatalf("%s: sample %d is %v", name, i, v)
			}
		}
		rms, peak, mean := stats(x)
		if rms < 0.02 || peak > 1 || math.Abs(mean) > 0.002 {
			t.Errorf("%s: rms %.4f, peak %.4f, mean %.5f", name, rms, peak, mean)
		}
	}
}

func TestSameSeedPlaysTheSame(t *testing.T) {
	s := songs(t)["mascot-dance"]
	a := play(synth.NewPlayer(s, 9), 3*48000)
	b := play(synth.NewPlayer(s, 9), 3*48000)
	if !slices.Equal(a, b) {
		t.Fatal("two players of the same seed played differently")
	}
}

func TestTiersGrowLouder(t *testing.T) {
	for name, s := range songs(t) {
		p := synth.NewPlayer(s, 3)
		if p.Look(0).Wander {
			continue
		}
		var last float64
		for tier := 1; tier <= p.Tiers(); tier++ {
			p.SetTier(tier)
			// The tier comes in at the next phrase.
			rms, _, _ := stats(play(p, phrase(p)))
			if tier > 1 && rms <= last {
				t.Errorf("%s: tier %d is no louder than the one below: %.4f, then %.4f", name, tier, last, rms)
			}
			last = rms
		}
	}
}

func TestPlayerIsABandPlayer(t *testing.T) {
	p := synth.NewPlayer(songs(t)["keypad-round"], 1)
	var bp band.Player = p
	if _, ok := bp.(band.Tiered); !ok {
		t.Error("not a band.Tiered")
	}
	if _, ok := bp.(band.Triggered); !ok {
		t.Error("not a band.Triggered")
	}
	w, ok := bp.(band.Watcher)
	if !ok {
		t.Fatal("not a band.Watcher")
	}
	play(p, 48000)
	st := w.Watch()
	if st.PhraseBars != 8 || len(st.Parts) != 8 {
		t.Fatalf("watched %+v", st)
	}
	if err := p.SetPart("nothing", band.PartOn); !errors.Is(err, band.ErrNoPart) {
		t.Errorf("SetPart of no part: %v", err)
	}
}

func TestPartsStartAndStopAtThePhrase(t *testing.T) {
	p := synth.NewPlayer(songs(t)["keypad-round"], 1)
	if err := p.SetPart("drums", band.PartOn); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPart("pad", band.PartOff); err != nil {
		t.Fatal(err)
	}
	play(p, 1000)
	playing := map[string]bool{}
	for _, tr := range p.Look(p.Played()).Tracks {
		playing[tr.Name] = tr.Playing
	}
	if !playing["drums"] || playing["pad"] || !playing["bass"] || playing["lead"] {
		t.Fatalf("playing %v", playing)
	}
}

func TestKeypadPlaysThePentatonic(t *testing.T) {
	p := synth.NewPlayer(songs(t)["keypad-round"], 1)
	play(p, 4800)
	for d := range 10 {
		p.Key(d)
	}
	play(p, 4800)
	var got []int
	for _, n := range p.Notes(nil, 0) {
		if n.Track == "keypad" {
			got = append(got, n.Pitch)
		}
	}
	slices.Sort(got)
	// C D E G A from C5, and on up.
	want := []int{72, 74, 76, 79, 81, 84, 86, 88, 91, 93}
	if !slices.Equal(got, want) {
		t.Fatalf("the keys played %v, want %v", got, want)
	}
}

func TestStingEndsTheSong(t *testing.T) {
	p := synth.NewPlayer(songs(t)["boss-entrance"], 1)
	play(p, 30000)
	if err := p.Sting("victory"); err != nil {
		t.Fatal(err)
	}
	if err := p.Sting("defeat"); !errors.Is(err, synth.ErrNoSting) {
		t.Errorf("Sting of none: %v", err)
	}
	l := p.Look(0)
	bar := int(l.BarFrames)
	sting := play(p, 6*bar)
	if rms, _, _ := stats(sting); rms < 0.02 {
		t.Errorf("the sting played at %.4f rms", rms)
	}
	if got := p.Look(p.Played()); got.Sting != "victory" && !got.Stopped {
		t.Errorf("looking at %+v", got)
	}
	// Its last chord rings out, and then there is silence.
	play(p, 6*48000)
	if rms, _, _ := stats(play(p, 48000)); rms > 1e-4 {
		t.Errorf("after the sting, %.5f rms", rms)
	}
	if !p.Look(p.Played()).Stopped {
		t.Error("the song goes on after the sting")
	}
}

func TestEditsTakeHoldAsItPlays(t *testing.T) {
	s := songs(t)["keypad-round"].Clone()
	p := synth.NewPlayer(s, 1)
	p.SetTier(4)
	play(p, phrase(p)+1000)
	edit := s.Clone()
	edit.Tracks[1].Pattern = "c0*8"
	edit.Patches["pad"].Filter.Cutoff = 300
	edit.Tracks = append(edit.Tracks, &synth.Track{Name: "new", Tier: 1, Patch: "bell", Pattern: "x*4", Arp: "up"})
	edit.Tracks = slices.DeleteFunc(edit.Tracks, func(t *synth.Track) bool { return t.Name == "chop" })
	if err := p.SetSong(edit); err != nil {
		t.Fatal(err)
	}
	x := play(p, 2*phrase(p))
	if rms, peak, _ := stats(x); rms < 0.02 || peak > 1 {
		t.Errorf("after the edit, rms %.4f, peak %.4f", rms, peak)
	}
	names := map[string]bool{}
	for _, tr := range p.Look(p.Played()).Tracks {
		names[tr.Name] = tr.Playing
	}
	if _, ok := names["chop"]; ok || !names["new"] {
		t.Errorf("after the edit, the tracks are %v", names)
	}
	bass := 0
	for _, n := range p.Notes(nil, p.Played()-int64(p.Look(0).BarFrames)) {
		if n.Track == "bass" {
			bass++
		}
	}
	if bass < 8 {
		t.Errorf("the bass played %d notes in its last bar, after its pattern became c0*8", bass)
	}
	bad := s.Clone()
	bad.Tracks[0].Pattern = "[ch"
	if err := p.SetSong(bad); err == nil {
		t.Error("a song of a broken pattern was taken")
	}
}

func TestBadSongsSayWhy(t *testing.T) {
	base := songs(t)["keypad-round"]
	for _, c := range []struct {
		edit func(s *synth.Song)
		want string
	}{
		{func(s *synth.Song) { s.Tracks[0].Patch = "nope" }, "patch \"nope\""},
		{func(s *synth.Song) { s.Tracks[0].Pattern = "ch zz" }, "\"zz\" is no note"},
		{func(s *synth.Song) { s.Tracks[2].Pattern = "bd snarez" }, "no drum \"snarez\""},
		{func(s *synth.Song) { s.Tracks[0].Params = map[string]string{"wobble": "1"} }, "wobble"},
		{func(s *synth.Song) { s.Chords = []string{"Hm"} }, "Hm"},
		{func(s *synth.Song) { s.Patches["pad"].Osc[0].Wave = "zap" }, "zap"},
		{func(s *synth.Song) { s.Tracks[0].Bars = 3 }, "3 bars"},
	} {
		s := base.Clone()
		c.edit(s)
		err := s.Check()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("got %v, want an error with %q", err, c.want)
		}
	}
}

func TestGeneratedSongs(t *testing.T) {
	s := songs(t)["mascot-dance"].Clone()
	s.Chords = nil
	s.Generate = &synth.Generate{Style: "kpop", Chords: 4, Seed: 5, Sevenths: 0.5, Evolve: 1}
	p := synth.NewPlayer(s, 2)
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	first := p.Look(0).Chords
	play(p, phrase(p)+1000)
	second := p.Look(p.Played()).Chords
	if len(first) != 4 || len(second) != 4 || slices.Equal(first, second) {
		t.Errorf("the progression evolved from %v to %v", first, second)
	}
}

// BenchmarkMascotDance makes a second of the busiest song, at its
// fullest, and reports how many times faster than it plays.
func BenchmarkMascotDance(b *testing.B) {
	s := songs(b)["mascot-dance"].Clone()
	for _, t := range s.Tracks {
		t.Core = true
	}
	p := synth.NewPlayer(s, 1)
	buf := make([]float32, 2*512)
	b.ResetTimer()
	start := time.Now()
	for range b.N {
		for range 48000 / 512 {
			_, _ = p.Read(buf)
		}
	}
	b.ReportMetric(float64(b.N)/time.Since(start).Seconds(), "x-realtime")
}

func TestPlayerIsSafeFromOtherGoroutines(t *testing.T) {
	s := songs(t)["keypad-round"]
	p := synth.NewPlayer(s.Clone(), 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			p.SetTier(i%4 + 1)
			p.Key(i % 10)
			_ = p.SetPart("pad", band.PartControl(i%3))
			_ = p.Look(p.Played())
			_ = p.Notes(nil, 0)
			_ = p.Hits(nil, p.Played(), p.Played()+48000)
			_ = p.Beat(p.Played())
			_ = p.Watch()
			if i%50 == 0 {
				edit := s.Clone()
				edit.Tracks[0].Gain = float64(-i % 12)
				_ = p.SetSong(edit)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	buf := make([]float32, 2*480)
	for {
		select {
		case <-done:
			return
		default:
			_, _ = p.Read(buf)
		}
	}
}

func TestMuteSoloMetersAndAuditions(t *testing.T) {
	s := songs(t)["keypad-round"].Clone()
	for _, tr := range s.Tracks {
		if tr.Name == "bass" {
			tr.Mute = true
		}
	}
	p := synth.NewPlayer(s, 1)
	p.WatchTrack("pad")
	play(p, 48000)
	levels := map[string]synth.TrackLook{}
	for _, tr := range p.Look(p.Played()).Tracks {
		levels[tr.Name] = tr
	}
	if l := levels["pad"]; l.Peak[0] <= 0.01 || l.RMS[1] <= 0.001 || l.RMS[0] > l.Peak[0] {
		t.Errorf("the pad's meter reads %+v", l)
	}
	if l := levels["bass"]; !l.Mute || l.Peak[0] != 0 {
		t.Errorf("the muted bass's meter reads %+v", l)
	}
	scope := p.Scope(make([]float32, 512))
	if rms, _, _ := stats(scope); len(scope) != 512 || rms < 0.001 {
		t.Errorf("the pad's scope holds %d frames at %.4f rms", len(scope), rms)
	}
	solo := s.Clone()
	for _, tr := range solo.Tracks {
		tr.Mute, tr.Solo = false, tr.Name == "bass"
	}
	if err := p.SetSong(solo); err != nil {
		t.Fatal(err)
	}
	// The pad's meter falls some 14 dB a second.
	play(p, 4*48000)
	for _, tr := range p.Look(p.Played()).Tracks {
		if tr.Name == "pad" && tr.Peak[0] > 0.01 {
			t.Errorf("the pad reads %v while the bass is soloed", tr.Peak[0])
		}
	}
	quiet := synth.NewPlayer(&synth.Song{BPM: 120, Patches: s.Patches, Tracks: []*synth.Track{{Name: "rest", Tier: 1, Patch: "pad", Pattern: "~"}}}, 1)
	quiet.Audition("lead", 72, 1, 0.3)
	quiet.AuditionDrum("kit", "sn", 1)
	if rms, _, _ := stats(play(quiet, 9600)); rms < 0.01 {
		t.Errorf("the auditions played at %.4f rms", rms)
	}
}

func TestPreviews(t *testing.T) {
	s := songs(t)["boss-entrance"]
	wave, err := synth.PreviewPatch(s.Patches["brass"], 60, 0.5, 1)
	if rms, _, _ := stats(wave); err != nil || len(wave) != 48000 || rms < 0.01 {
		t.Errorf("the brass's preview: %v, %d frames, %.4f rms", err, len(wave), rms)
	}
	hit, err := synth.PreviewDrum(s.Patches["orch"], "tim", 1)
	if rms, _, _ := stats(hit); err != nil || rms < 0.01 {
		t.Errorf("the timpani's preview: %v, %.4f rms", err, rms)
	}
	if _, err := synth.PreviewDrum(s.Patches["orch"], "zap", 1); err == nil {
		t.Error("a drum of no kit previewed")
	}
}

func TestAVictoryStingLastsTwoBarsAndEndsOnItsChord(t *testing.T) {
	// The game's results screen counts its stars up in about two bars:
	// the fanfare ends with them, on a chord held to its end.
	for _, name := range []string{"boss-entrance", "alien-entrance"} {
		p := synth.NewPlayer(songs(t)[name], 1)
		play(p, 30000)
		l := p.Look(0)
		bar := l.BarFrames
		if err := p.Sting("victory"); err != nil {
			t.Fatal(err)
		}
		var out []float32
		frames := 0
		for !p.Look(p.Played()).Stopped {
			out = append(out, play(p, 480)...)
			frames += 480
			if frames > int(4*bar) {
				t.Fatalf("%s: the sting plays on past 4 bars", name)
			}
		}
		// It starts on the next beat, and lasts two bars.
		beat := bar / float64(l.BeatsPerBar)
		if got := float64(frames); got < 2*bar || got > 2*bar+beat+480 {
			t.Errorf("%s: the sting lasts %.2f s, not two bars, %.2f s", name, got/48000, 2*bar/48000)
		}
		// Its last quarter still sounds: the final chord, held.
		end, _, _ := stats(out[len(out)-int(bar/2):])
		if end < 0.02 {
			t.Errorf("%s: the sting's last half bar is %.4f rms, not its final chord", name, end)
		}
		// Then its chord rings out into silence, through the reverb's
		// tail.
		play(p, 6*48000)
		if rms, _, _ := stats(play(p, 48000)); rms > 3e-4 {
			t.Errorf("%s: after the sting, %.5f rms, not under -70 dB", name, rms)
		}
	}
}
