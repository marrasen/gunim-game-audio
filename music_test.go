package music

import (
	"testing"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
)

func TestTheSongsPartsAreAllThereCutAtTheirBars(t *testing.T) {
	s := Song()
	phrase := s.BarAt(16)
	solos := 0
	for _, p := range s.Parts {
		want := map[band.Piece]int64{band.Intro: phrase, band.Loop: phrase, band.Outro: s.BarAt(4)}
		if p.Solo {
			solos++
			want = map[band.Piece]int64{band.Solo: s.BarAt(20)}
		}
		for piece, n := range want {
			src, err := p.Open(piece)
			if err != nil {
				t.Fatalf("%s's %s: %v", p.Name, piece, err)
			}
			sk, ok := src.(audio.Seeker)
			if !ok {
				t.Fatalf("%s's %s decodes to a source of no length", p.Name, piece)
			}
			if got := sk.Len(); got < n-1 || got > n+1 {
				t.Fatalf("%s's %s is %d frames, want %d", p.Name, piece, got, n)
			}
		}
	}
	if len(s.Parts) != 10 || solos != 1 {
		t.Fatalf("%d parts, %d of them solos; want 10 and 1", len(s.Parts), solos)
	}
}

func TestTheBandPlaysTheSong(t *testing.T) {
	b := New(1)
	buf := make([]float32, 2*audio.SampleRate)
	peak := float32(0)
	for range 4 {
		if _, err := b.Read(buf); err != nil {
			t.Fatal(err)
		}
		for _, v := range buf {
			peak = max(peak, v, -v)
		}
	}
	if peak < 0.05 {
		t.Fatalf("four seconds of the band peak at %v; want music", peak)
	}
}
