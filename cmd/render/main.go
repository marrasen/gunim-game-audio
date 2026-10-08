// Command render renders a song of the library to a WAV file, and
// says how loud it is: its integrated loudness, in LUFS, and its peak,
// tier by tier, and where -tracks asks, each track's alone. It is for
// mixing a song made in code without a speaker to hand.
//
//	go run ./cmd/render -song keypad-round -o round.wav
//
// A tiers song plays each tier for -phrases phrases, from tier 1 up, and
// then the stings, one after another; any other song plays -phrases
// phrases in all.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/synth"
)

func main() {
	name := flag.String("song", "", "the song to render, by name")
	out := flag.String("o", "", "the WAV file to write; none writes no file")
	phrases := flag.Int("phrases", 1, "how many phrases each tier plays")
	tracks := flag.Bool("tracks", false, "measure each track alone too")
	bands := flag.Bool("bands", false, "with -tracks, show how loud each track is in each octave")
	seed := flag.Uint64("seed", 1, "how the band plays the song")
	flag.Parse()
	s, err := music.Song(*name)
	if err != nil {
		log.Fatal(err)
	}
	song, ok := s.(*synth.Song)
	if !ok {
		log.Fatalf("%s is no song made in code", *name)
	}
	if err := render(song, *out, *phrases, *seed); err != nil {
		log.Fatal(err)
	}
	if *tracks {
		if *bands {
			fmt.Printf("  %-18s %37s", "", "")
			for _, f := range octaves {
				fmt.Printf(" %5s", label(f))
			}
			fmt.Println()
		}
		for _, t := range song.Tracks {
			one := song.Clone()
			one.Tracks = nil
			for _, u := range song.Clone().Tracks {
				if u.Name == t.Name {
					u.Tier, u.Core = 1, true
					one.Tracks = append(one.Tracks, u)
				}
			}
			// The track alone: no other track for the mix to duck to,
			// or a dancer to clap to.
			one.Mode, one.Mix.Duck, one.Stings, one.Mix.Transitions, one.Clap = "tiers", "", nil, nil, nil
			lufs, peak, oct := measure(one, 2, *seed)
			fmt.Printf("  %-18s %6.1f LUFS  peak %6.1f dB", t.Name, lufs, peak)
			if *bands {
				for _, b := range oct {
					fmt.Printf(" %5.0f", b)
				}
			}
			fmt.Println()
		}
	}
}

// render renders song, tier by tier, to file, and prints each tier's
// loudness.
func render(song *synth.Song, file string, phrases int, seed uint64) (err error) {
	p := synth.NewPlayer(song, seed)
	if perr := p.Err(); perr != nil {
		return perr
	}
	var w *audio.WAVWriter
	if file != "" {
		f, cerr := os.Create(file)
		if cerr != nil {
			return cerr
		}
		defer func() { err = errors.Join(err, f.Close()) }()
		if w, err = audio.NewWAVWriter(f, audio.SampleRate, 24, true); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, w.Close()) }()
	}
	look := p.Look(0)
	phrase := int(float64(look.PhraseBars) * look.BarFrames)
	tiers := max(look.Tiers, 1)
	if look.Wander {
		tiers = 1
	}
	buf := make([]float32, 2*1024)
	play := func(frames int, label string) error {
		var m audio.LoudnessMeter
		for done := 0; done < frames; done += len(buf) / 2 {
			if _, err := p.Read(buf); err != nil {
				return err
			}
			m.Write(buf)
			if w != nil {
				if err := w.Write(buf); err != nil {
					return err
				}
			}
		}
		lufs, _ := m.Integrated()
		fmt.Printf("%-24s %6.1f LUFS  peak %6.1f dB\n", label, lufs, 20*math.Log10(float64(m.Peak())+1e-9))
		return nil
	}
	// Each tier is asked for a bar before its phrase, as the player
	// chooses a phrase's parts as it starts.
	bar := int(look.BarFrames)
	p.SetTier(1)
	for tier := 1; tier <= tiers; tier++ {
		label := fmt.Sprintf("%s, tier %d", song.Title, tier)
		if look.Wander {
			label = song.Title
		}
		if err := play(phrases*phrase-bar, label); err != nil {
			return err
		}
		p.SetTier(tier + 1)
		if err := play(bar, "  its last bar"); err != nil {
			return err
		}
	}
	for _, name := range look.Stings {
		if err := p.Sting(name); err != nil {
			return err
		}
		if err := play(phrase, "sting "+name); err != nil {
			return err
		}
	}
	return nil
}

// octaves are the middles of the octaves -bands shows.
var octaves = []float64{31, 63, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

func label(f float64) string {
	if f >= 1000 {
		return fmt.Sprintf("%gk", f/1000)
	}
	return fmt.Sprint(f)
}

// bandpass is a filter of an octave about a frequency, as RBJ's
// cookbook gives it.
type bandpass struct {
	b0, b2, a1, a2 float64
	x1, x2, y1, y2 float64
	sum            float64
}

func newBandpass(f float64) *bandpass {
	w := 2 * math.Pi * f / audio.SampleRate
	alpha := math.Sin(w) * math.Sinh(math.Ln2/2*w/math.Sin(w))
	a0 := 1 + alpha
	return &bandpass{b0: alpha / a0, b2: -alpha / a0, a1: -2 * math.Cos(w) / a0, a2: (1 - alpha) / a0}
}

func (b *bandpass) step(x float64) {
	y := b.b0*x + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	b.sum += y * y
}

// measure returns the loudness and peak of song's first phrases, and
// how loud each of octaves is, in decibels.
func measure(song *synth.Song, phrases int, seed uint64) (lufs, peak float64, oct []float64) {
	p := synth.NewPlayer(song, seed)
	if p.Err() != nil {
		return math.Inf(-1), math.Inf(-1), nil
	}
	bps := make([]*bandpass, len(octaves))
	for i, f := range octaves {
		bps[i] = newBandpass(f)
	}
	var t band.Tiered = p
	t.SetTier(1)
	look := p.Look(0)
	frames := int(float64(phrases*look.PhraseBars) * look.BarFrames)
	buf := make([]float32, 2*1024)
	var m audio.LoudnessMeter
	n := 0
	for done := 0; done < frames; done += len(buf) / 2 {
		_, _ = p.Read(buf)
		m.Write(buf)
		for i := 0; i < len(buf); i += 2 {
			x := float64(buf[i]+buf[i+1]) / 2
			for _, b := range bps {
				b.step(x)
			}
			n++
		}
	}
	lufs, _ = m.Integrated()
	for _, b := range bps {
		oct = append(oct, 10*math.Log10(b.sum/float64(n)+1e-20))
	}
	return lufs, 20 * math.Log10(float64(m.Peak())+1e-9), oct
}
