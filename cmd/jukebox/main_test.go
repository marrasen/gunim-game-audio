package main

import (
	"testing"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"

	music "github.com/marrasen/gunim-game-audio"
)

// tieredSong returns a song of the library that plays in tiers.
func tieredSong(t *testing.T) band.Song {
	t.Helper()
	for _, name := range music.Songs() {
		s, err := music.Song(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Play(1).(band.Tiered); ok {
			return s
		}
	}
	t.Skip("the library holds no tiered song")
	return nil
}

// TestAutoplayStepsATierEachPhrase plays a tiered song with autoplay on,
// checking after every 10 ms of sound, and finds each phrase played at
// the tier after the last one's, from the top tier back to tier 1.
func TestAutoplayStepsATierEachPhrase(t *testing.T) {
	pl := &player{p: tieredSong(t).Play(1), auto: true, phrase: -1}
	tp, ok := pl.p.(band.Tiered)
	w, ok2 := pl.p.(band.Watcher)
	if !ok || !ok2 {
		t.Fatal("the tiered song's player is no band.Tiered and band.Watcher")
	}
	n := tp.Tiers()
	if n < 2 {
		t.Skipf("the song has %d tier", n)
	}
	buf := make([]float32, 2*audio.SampleRate/100)
	pl.autoplay()
	// The tier set during each phrase, by phrase.
	set := map[int]int{0: tp.Tier()}
	phrases := 2*n + 1
	for {
		if _, err := pl.p.Read(buf); err != nil {
			t.Fatal(err)
		}
		pl.autoplay()
		st := w.Watch()
		phrase := st.Bar / st.PhraseBars
		if phrase >= phrases {
			break
		}
		if got, ok := set[phrase]; ok && got != tp.Tier() {
			t.Fatalf("phrase %d: the tier moved from %d to %d within the phrase", phrase, got, tp.Tier())
		}
		set[phrase] = tp.Tier()
	}
	for p := 1; p < phrases; p++ {
		if want := set[p-1]%n + 1; set[p] != want {
			t.Errorf("phrase %d set tier %d, want %d after %d", p, set[p], want, set[p-1])
		}
	}
}

// TestAutoplayStopsAtATierChosen checks autoplay leaves the tier alone
// once it is off.
func TestAutoplayStopsAtATierChosen(t *testing.T) {
	pl := &player{p: tieredSong(t).Play(1), phrase: -1}
	tp, ok := pl.p.(band.Tiered)
	if !ok {
		t.Fatal("the tiered song's player is no band.Tiered")
	}
	tp.SetTier(1)
	buf := make([]float32, 2*audio.SampleRate/100)
	for range 3000 {
		if _, err := pl.p.Read(buf); err != nil {
			t.Fatal(err)
		}
		pl.autoplay()
		if tp.Tier() != 1 {
			t.Fatalf("autoplay off moved the tier to %d", tp.Tier())
		}
	}
}
