package music

import (
	"errors"
	"slices"
	"testing"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
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

func TestEveryWandersPartsAreThereCutAtItsBars(t *testing.T) {
	for _, name := range Songs() {
		s, err := Song(name)
		if err != nil {
			t.Fatal(err)
		}
		w, ok := s.(*band.Wander)
		if !ok {
			continue
		}
		bars := w.PhraseBars
		if bars == 0 {
			bars = 16
		}
		phrase := w.BarAt(bars)
		looping := 0
		for _, p := range w.Parts {
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
					if n < w.BarAt(1)/2 {
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

func TestSongSaysWhenThereIsNoSuchSong(t *testing.T) {
	if _, err := Song("no-such-song"); !errors.Is(err, ErrNoSong) {
		t.Fatalf("Song returned %v, want ErrNoSong", err)
	}
}
