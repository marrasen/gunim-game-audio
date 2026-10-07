package synth_test

import (
	"math"
	"testing"

	"github.com/marrasen/gunim/audio/band"

	"github.com/marrasen/gunim-music/synth"
)

// sighting is a hit, and the frame the player had made when Hits first
// told of it.
type sighting struct {
	h  synth.Hit
	at int64
}

// watch plays bars bars of p, asking Hits after each 10 ms what is
// coming, and returns each hit as first told of, and the hits that
// sounded. before, if set, runs before each 10 ms is made.
func watch(p *synth.Player, bars int, before func(made int64)) (told []sighting, sounded []synth.Hit) {
	bar := p.Look(0).BarFrames
	end := int64(float64(bars) * bar)
	buf := make([]float32, 2*480)
	for p.Played() < end {
		if before != nil {
			before(p.Played())
		}
		_, _ = p.Read(buf)
		made := p.Played()
		for _, h := range p.Hits(nil, made, made+int64(3*bar)) {
			if find(told, h) < 0 {
				told = append(told, sighting{h, made})
			}
		}
	}
	for _, h := range p.Hits(nil, 0, end) {
		if !h.Foreseen {
			sounded = append(sounded, h)
		}
	}
	return told, sounded
}

// find returns the index in told of h, the same drum of the same track
// within 4 ms, as a human's nudge moves it, or -1.
func find(told []sighting, h synth.Hit) int {
	for i, s := range told {
		if s.h.Track == h.Track && s.h.Drum == h.Drum && math.Abs(float64(s.h.Frame-h.Frame)) < 200 {
			return i
		}
	}
	return -1
}

func TestEveryClapIsToldOfABarBeforeItSounds(t *testing.T) {
	// Every song made in code claps, and tells of each clap in time.
	for name, song := range songs(t) {
		p := song.Play(1).(*synth.Player)
		p.SetTier(4)
		// A wander song's claps come and go; the game holds them in.
		_ = p.SetPart("clap", band.PartOn)
		_ = p.SetPart("claps", band.PartOn)
		l := p.Look(0)
		told, sounded := watch(p, 2*l.PhraseBars, nil)
		claps := 0
		for _, h := range sounded {
			bar := int(float64(h.Frame) / l.BarFrames)
			if h.Kind != synth.Clap || bar == 0 || (l.Wander && bar%l.PhraseBars == 0) {
				// The first bar is told of only as it starts, and a
				// wander song's phrase chooses its parts as it starts.
				continue
			}
			claps++
			i := find(told, h)
			if i < 0 {
				t.Fatalf("%s: the clap at %d was never told of", name, h.Frame)
			}
			if lead := float64(h.Frame - told[i].at); lead < l.BarFrames {
				t.Errorf("%s: the clap at %d was told of only %.0f ms before it sounded", name, h.Frame, lead/48)
			}
		}
		if claps < 8 {
			t.Errorf("%s: %d claps sounded in 2 phrases", name, claps)
		}
	}
}

func TestAForeseenHitSoundsAsForeseen(t *testing.T) {
	// A tiers song held at its tier changes nothing: every hit foreseen
	// sounds, of its kind, within a human's nudge of when it was told.
	p := songs(t)["keypad-round"].Play(2).(*synth.Player)
	p.SetTier(4)
	l := p.Look(0)
	told, sounded := watch(p, 2*l.PhraseBars, nil)
	end := int64(float64(2*l.PhraseBars) * l.BarFrames)
	for _, s := range told {
		if !s.h.Foreseen || s.h.Frame >= end {
			continue
		}
		ok := false
		for _, h := range sounded {
			if h.Track == s.h.Track && h.Drum == s.h.Drum && h.Kind == s.h.Kind && math.Abs(float64(h.Frame-s.h.Frame)) < 200 {
				ok = true
			}
		}
		if !ok {
			t.Errorf("the %s of %s foreseen at %d never sounded", s.h.Drum, s.h.Track, s.h.Frame)
		}
	}
}

func TestATierAskedForBeforeThePhrasesLastBarIsForeseen(t *testing.T) {
	// Keypad Round's claps come in at tier 2. Asked for in the phrase's
	// second-last bar, they are told of a bar before they sound.
	p := songs(t)["keypad-round"].Play(3).(*synth.Player)
	p.SetTier(1)
	l := p.Look(0)
	ask := int64(float64(l.PhraseBars-2) * l.BarFrames)
	told, sounded := watch(p, l.PhraseBars+2, func(made int64) {
		if made >= ask && p.Tier() == 1 {
			p.SetTier(2)
		}
	})
	first := -1
	for i, h := range sounded {
		if h.Kind == synth.Clap {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("no clap sounded after the tier rose")
	}
	h := sounded[first]
	if bar := int(float64(h.Frame) / l.BarFrames); bar != l.PhraseBars {
		t.Errorf("the first clap sounded in bar %d, not the phrase's start, %d", bar, l.PhraseBars)
	}
	if i := find(told, h); i < 0 || float64(h.Frame-told[i].at) < l.BarFrames {
		t.Errorf("the first clap at the new tier was not told of a bar ahead")
	}
}

func TestTheBeatCountsTheBarsBeats(t *testing.T) {
	p := songs(t)["mascot-dance"].Play(1).(*synth.Player)
	l := p.Look(0)
	play(p, int(3.5*l.BarFrames))
	beat := l.BarFrames / float64(l.BeatsPerBar)
	at := int64(2*l.BarFrames + 1.5*beat)
	b := p.Beat(at)
	if b.Bar != 2 || b.Beat != 1 || math.Abs(b.Phase-0.5) > 0.01 || b.BeatsPerBar != l.BeatsPerBar {
		t.Fatalf("1.5 beats into bar 2 the beat is %+v", b)
	}
	if math.Abs(float64(b.Frame)-(2*l.BarFrames+beat)) > 2 || math.Abs(b.Frames-beat) > 1e-6 {
		t.Errorf("the beat started on %d and lasts %.1f frames", b.Frame, b.Frames)
	}
	// Each beat follows the last, its phase rising and starting over.
	prev := p.Beat(int64(2 * l.BarFrames))
	for f := int64(2 * l.BarFrames); f < int64(3.4*l.BarFrames); f += 100 {
		b := p.Beat(f)
		n, m := prev.Bar*prev.BeatsPerBar+prev.Beat, b.Bar*b.BeatsPerBar+b.Beat
		if m < n || m > n+1 || (m == n && b.Phase < prev.Phase) {
			t.Fatalf("at %d the beat went from %+v to %+v", f, prev, b)
		}
		prev = b
	}
}
