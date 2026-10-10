package music

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"

	"github.com/marrasen/gunim-game-audio/synth"
)

func TestEverySongLoadsAndNamesItsMaker(t *testing.T) {
	names := Songs()
	if !slices.Contains(names, GreekThemes) {
		t.Fatalf("the library holds %v, without %s", names, GreekThemes)
	}
	for _, name := range names {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		if i := s.Info(); i.Title == "" || i.Artist == "" || i.BPM <= 0 {
			t.Fatalf("%s's song.json is short of something: %+v", name, i)
		}
	}
}

// cut returns a song's parts, and where its bars start and how many
// make a phrase, for the kinds of song the library holds cut from
// recordings, and false for a song made in code, which package synth
// tests.
func cut(t *testing.T, s band.Song) (parts []band.Part, barAt func(int) int64, phraseBars int, ok bool) {
	t.Helper()
	switch s := s.(type) {
	case *band.Wander:
		return s.Parts, s.BarAt, cmp.Or(s.PhraseBars, 16), true
	case *band.Tiers:
		return s.Parts, s.BarAt, cmp.Or(s.PhraseBars, 8), true
	case *synth.Song:
		return nil, nil, 0, false
	}
	t.Fatalf("a song of type %T", s)
	return nil, nil, 0, false
}

func TestEverySongsPartsAreThereCutAtItsBars(t *testing.T) {
	for _, name := range Songs() {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		parts, barAt, bars, ok := cut(t, s)
		if !ok {
			continue
		}
		phrase := barAt(bars)
		looping := 0
		for _, p := range parts {
			pieces := []band.Piece{band.Intro, band.Loop, band.Outro}
			if p.Solo {
				pieces = []band.Piece{band.Solo}
			} else {
				looping++
			}
			for _, piece := range pieces {
				src, err := p.Open(piece)
				if err != nil {
					t.Fatalf("%s: %s's %s: %v", name, p.Name, piece, err)
				}
				sk, ok := src.(audio.Seeker)
				if !ok {
					t.Fatalf("%s: %s's %s decodes to a source of no length", name, p.Name, piece)
				}
				n := sk.Len()
				switch piece {
				case band.Intro, band.Loop:
					if n < phrase-1 || n > phrase+1 {
						t.Fatalf("%s: %s's %s is %d frames, want a phrase, %d", name, p.Name, piece, n, phrase)
					}
				default:
					if n < barAt(1)/2 {
						t.Fatalf("%s: %s's %s is %d frames, under half a bar", name, p.Name, piece, n)
					}
				}
			}
		}
		if looping < 2 {
			t.Fatalf("%s has %d parts that loop; want two at least", name, looping)
		}
	}
}

func TestGreekThemesIsTenSynthsAndASolo(t *testing.T) {
	s, err := Song(GreekThemes)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := s.(*band.Wander)
	if !ok {
		t.Fatalf("Greek Themes is a %T, want a *band.Wander", s)
	}
	solos := 0
	for _, p := range w.Parts {
		if p.Solo {
			solos++
		}
	}
	if len(w.Parts) != 10 || solos != 1 {
		t.Fatalf("%d parts, %d of them solos; want 10 and 1", len(w.Parts), solos)
	}
}

func TestARoundSongPlaysInFourTiers(t *testing.T) {
	s, err := Song(ARoundSong)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.Play(1).(band.Tiered)
	if !ok {
		t.Fatalf("A round song's player is a %T, want a band.Tiered", s.Play(1))
	}
	w, ok := p.(band.Watcher)
	if !ok {
		t.Fatal("A round song's player is no band.Watcher")
	}
	if p.Tiers() != 4 || p.Tier() != 1 {
		t.Fatalf("%d tiers, at tier %d; want 4, at 1", p.Tiers(), p.Tier())
	}
	var playing []string
	for _, ps := range w.Watch().Parts {
		if ps.Playing {
			playing = append(playing, ps.Name)
		}
	}
	slices.Sort(playing)
	if !slices.Equal(playing, []string{"bass", "ensemble-pad"}) {
		t.Fatalf("tier 1 plays %v, want the bass and the pad", playing)
	}
}

func TestEverySongPlays(t *testing.T) {
	for _, name := range Songs() {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		p := s.Play(1)
		buf := make([]float32, 2*audio.SampleRate)
		peak := float32(0)
		for range 4 {
			if _, err := p.Read(buf); err != nil {
				t.Fatal(err)
			}
			for _, v := range buf {
				peak = max(peak, v, -v)
			}
		}
		if peak < 0.05 {
			t.Fatalf("four seconds of %s peak at %v; want music", name, peak)
		}
	}
}

func TestTheSongsMadeInCodeAreThere(t *testing.T) {
	for _, name := range []string{KeypadRound, BossEntrance, MascotDance, BubbleBounce, SisterDreams, GraveyardGallop,
		PocketKingdom, MeadowHop, HerosField, PalaceRun, UnderworldAscent, StarDrift, OrbitRound, AlienEntrance,
		CandyClouds, CompassRose, SummerMeadow, TinkerLab, TinkerRound, NotteDiNeon, RingMeTwice, WireCathedral, MirrorShine, Meridian, Parallax, PhantomHighway} {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.(*synth.Song); !ok {
			t.Fatalf("%s is a %T, want a *synth.Song", name, s)
		}
	}
}

// loudness plays bars of song name, made in code, at tier, and says how
// loud they are, in LUFS, as cmd/render measures a song.
func loudness(t *testing.T, name string, tier, bars int) float64 {
	t.Helper()
	s, err := Song(name)
	if err != nil {
		t.Fatal(err)
	}
	p := synth.NewPlayer(s.(*synth.Song), 1)
	p.SetTier(tier)
	look := p.Look(0)
	// A tier asked for starts at the next phrase: play the first, then
	// measure.
	skip := 0
	if tier > 1 {
		skip = int(float64(look.PhraseBars) * look.BarFrames)
	}
	frames := skip + int(float64(bars)*look.BarFrames)
	buf := make([]float32, 2*1024)
	var m audio.LoudnessMeter
	for done := 0; done < frames; done += len(buf) / 2 {
		if _, err := p.Read(buf); err != nil {
			t.Fatal(err)
		}
		if done >= skip {
			m.Write(buf)
		}
	}
	lufs, _ := m.Integrated()
	return lufs
}

func TestLabbetsSongsAreAsLoudAsTheOtherRooms(t *testing.T) {
	if testing.Short() {
		t.Skip("plays a minute of music")
	}
	// The room song sits with Candy Clouds and Compass Rose, within a
	// decibel of the range they span.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, name := range []string{CandyClouds, CompassRose} {
		l := loudness(t, name, 1, 16)
		lo, hi = min(lo, l), max(hi, l)
	}
	l := loudness(t, TinkerLab, 1, 16)
	t.Logf("Tinker Lab %.1f LUFS; Candy Clouds and Compass Rose %.1f to %.1f", l, lo, hi)
	if l < lo-1 || l > hi+1 {
		t.Errorf("Tinker Lab is %.1f LUFS, not within a decibel of %.1f to %.1f", l, lo, hi)
	}
	// The round song's full tier sits with Keypad Round's and Orbit
	// Round's.
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, name := range []string{KeypadRound, OrbitRound} {
		l := loudness(t, name, 4, 8)
		lo, hi = min(lo, l), max(hi, l)
	}
	l = loudness(t, TinkerRound, 4, 8)
	t.Logf("Tinker Round's tier 4 %.1f LUFS; Keypad Round's and Orbit Round's %.1f to %.1f", l, lo, hi)
	if l < lo-1 || l > hi+1 {
		t.Errorf("Tinker Round's tier 4 is %.1f LUFS, not within a decibel of %.1f to %.1f", l, lo, hi)
	}
}

func TestTheGroovesAreAsLoudAsEachOther(t *testing.T) {
	if testing.Short() {
		t.Skip("plays ten minutes of music")
	}
	// Each whole song sits where the mid-1980s' records did, -16 to
	// -14, and within a decibel and a half of Mascot Dance, the other
	// groove; the futurepop songs, louder on record, are held there too,
	// so a game's volume suits them all.
	m := loudness(t, MascotDance, 1, 16)
	for name, bars := range map[string]int{NotteDiNeon: 64, RingMeTwice: 80, Meridian: 96, Parallax: 96} {
		l := loudness(t, name, 1, bars)
		t.Logf("%s %.1f LUFS; Mascot Dance %.1f", name, l, m)
		if l < -16 || l > -14 || math.Abs(l-m) > 1.5 {
			t.Errorf("%s is %.1f LUFS; want -16 to -14, and within 1.5 dB of Mascot Dance's %.1f", name, l, m)
		}
	}
}

func TestTheLoudSongsAreLouderThanTheRest(t *testing.T) {
	if testing.Short() {
		t.Skip("plays six minutes of music")
	}
	// Industrial, club techno and noise pop are mastered louder than the
	// rest of the library: each whole song from -15 to -12 LUFS, short of their
	// records' -12 to -9, so a game's volume suits them all.
	for name, bars := range map[string]int{WireCathedral: 64, MirrorShine: 128, PhantomHighway: 96} {
		l := loudness(t, name, 1, bars)
		t.Logf("%s %.1f LUFS", name, l)
		if l < -15 || l > -12 {
			t.Errorf("%s is %.1f LUFS; want -15 to -12", name, l)
		}
	}
}

func TestSongSaysWhenThereIsNoSuchSong(t *testing.T) {
	if _, err := Song("no-such-song"); !errors.Is(err, ErrNoSong) {
		t.Fatalf("Song returned %v, want ErrNoSong", err)
	}
}

func TestTheCompanionsCallsLoadFromTheLibrary(t *testing.T) {
	lib, err := Calls()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"groda", "uggla", "enhorningskatt", "kpop-tjej", "kpop-kille",
		"robo-ninja", "trollkarlen", "drakungen", "fotbollsstjarnan", "raven", "whizpah"} {
		c := lib.Companion(id)
		if c == nil || len(c.Calls) != 3 {
			t.Errorf("%s makes %v", id, c)
		}
	}
}

func TestEverySongTellsItsBeat(t *testing.T) {
	for _, name := range Songs() {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		p := s.Play(1)
		// Play two bars and a half, then ask about a beat and a half into
		// the second bar.
		frames := int(audio.SampleRate * 60 / s.Info().BPM * 10)
		buf := make([]float32, 2*1000)
		for done := 0; done < frames; done += 1000 {
			_, _ = p.Read(buf)
		}
		b := BeatAt(s, p, 0)
		if b.Bar != 0 || b.Beat != 0 || b.Frames <= 0 {
			t.Errorf("%s: at its start the beat is %+v", name, b)
			continue
		}
		at := int64(float64(b.BeatsPerBar)*b.Frames + 1.5*b.Frames)
		b = BeatAt(s, p, at)
		if b.Bar != 1 || b.Beat != 1 || math.Abs(b.Phase-0.5) > 0.01 {
			t.Errorf("%s: 1.5 beats into its second bar the beat is %+v", name, b)
		}
		if want := 60 / s.Info().BPM * audio.SampleRate; math.Abs(b.Frames-want) > 1 {
			t.Errorf("%s: a beat lasts %.1f frames at %v BPM, not %.1f", name, b.Frames, s.Info().BPM, want)
		}
	}
}
