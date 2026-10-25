package plugsynth

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugtest"
)

// kitTestNotes are the drums each kit's test plays, by General MIDI's
// notes: a kick, a snare, a closed and an open hat, and a crash.
var kitTestNotes = []int{36, 38, 42, 46, 49}

// kitRoles are the names a song's pattern plays each of kitTestNotes'
// drums by, in any kit: the built-in drum's name first.
var kitRoles = map[int][]string{
	36: {"bd", "bd9", "sbd", "nbd"},
	38: {"sn", "ssn", "nsn"},
	42: {"hh", "shh", "nhh"},
	46: {"oh", "soh", "noh"},
	49: {"cr"},
}

// patternDrum returns the name a song's pattern plays General MIDI's
// note key by in kit: its number, for a kit of numbered drums, as
// GMKit's; the one of the note's role names the kit lists; or the
// built-in drum's.
func patternDrum(t *testing.T, kit map[string]synth.Drum, key int) string {
	t.Helper()
	if _, ok := kit[strconv.Itoa(key)]; ok {
		return strconv.Itoa(key)
	}
	var listed []string
	for _, n := range kitRoles[key] {
		if _, ok := kit[n]; ok {
			listed = append(listed, n)
		}
	}
	switch len(listed) {
	case 0:
		return kitRoles[key][0]
	case 1:
		return listed[0]
	}
	t.Fatalf("the kit lists %v for note %d: which one plays it is unclear", listed, key)
	return ""
}

// bandHz are the bands a drum's spectrum is measured in: an octave
// each, by their centres.
var bandHz = []float64{50, 100, 200, 400, 800, 1600, 3200, 6400, 12800}

// spans are the stretches a drum's level is measured over, in frames
// from its onset at 48 kHz: its attack, its body, and its ring.
var spans = [][2]int{{0, 960}, {960, 3840}, {3840, 12000}, {12000, 28800}}

// fingerprint returns how drum x, at 48 kHz, sounds, in decibels: its
// level in each of bandHz' octaves over its first half second, and then
// its level over each of spans.
func fingerprint(x []float32) []float64 {
	x = x[max(onset(x), 0):]
	head := x[:min(len(x), 24000)]
	out := make([]float64, 0, len(bandHz)+len(spans))
	for _, c := range bandHz {
		// Sixteen frequencies across the octave, by Goertzel's sum.
		var e float64
		for i := range 16 {
			f := c * math.Exp2((float64(i)+0.5)/16-0.5)
			k := 2 * math.Cos(2*math.Pi*f/48000)
			var s1, s2 float64
			for _, v := range head {
				s1, s2 = float64(v)+k*s1-s2, s1
			}
			e += s1*s1 + s2*s2 - k*s1*s2
		}
		out = append(out, 10*math.Log10(e/float64(len(head))+1e-12))
	}
	for _, sp := range spans {
		r := rms(x[min(sp[0], len(x)):min(sp[1], len(x))])
		out = append(out, 20*math.Log10(r+1e-6))
	}
	return out
}

// kitSound plays General MIDI's note key, at full velocity, through a
// fresh synth playing s, and returns its sound in mono, as PreviewDrum
// renders a drum.
func kitSound(t *testing.T, s Sound, key int) []float32 {
	t.Helper()
	p, _ := newSynth(t)
	if err := p.SetState(s.State()); err != nil {
		t.Fatalf("%s/%s/%s: %v", s.Bank, s.Group, s.Name, err)
	}
	setup := plug.Setup{Rate: 48000, MaxBlock: 512, Channels: 2}
	out := plugtest.Play(t, p.Processor(), setup, 36000, params(),
		plug.Event{Frame: 0, Kind: plug.NoteOn, Note: 1, Pitch: float64(key), Velocity: 1})
	mono := make([]float32, len(out[0]))
	for i := range mono {
		mono[i] = (out[0][i] + out[1][i]) / 2
	}
	return mono
}

// kits returns every drum kit the plugin offers.
func kits() []Sound {
	var out []Sound
	for _, s := range Sounds() {
		if s.patch.Kind == "drums" {
			out = append(out, s)
		}
	}
	return out
}

// TestEachKitPlaysAsItsSongsDo plays each kit's kick, snare, hats and
// crash through the plugin, and the drum a song's pattern plays by name
// in the same kit, and holds their spectra alike, octave by octave.
func TestEachKitPlaysAsItsSongsDo(t *testing.T) {
	ks := kits()
	if lib := len(slices.DeleteFunc(slices.Clone(ks), func(s Sound) bool { return s.Bank != Library })); lib != 12 {
		t.Errorf("the library offers %d kits, not 12", lib)
	}
	for _, s := range ks {
		for _, key := range kitTestNotes {
			name := patternDrum(t, s.patch.Kit, key)
			want, err := synth.PreviewDrum(s.patch, name, 0.75)
			if err != nil {
				t.Fatalf("%s/%s: %v", s.Group, s.Name, err)
			}
			// A synth's first hit starts its noise as a song's first hit
			// does, so the two sound alike to a fraction of a decibel.
			got, ws := fingerprint(kitSound(t, s, key)), fingerprint(want)
			if d := apart(got, ws); d > 0.5 {
				t.Errorf("%s/%s/%s: note %d sounds %.1f dB apart from the song's %s:\n%.1f\n%.1f",
					s.Bank, s.Group, s.Name, key, d, name, got, ws)
			}
		}
	}
}

// apart returns how far apart, in decibels, the spectra a and b are:
// the root of the mean square of their bands' differences, in the measures
// within 30 dB of either's loudest.
func apart(a, b []float64) float64 {
	top := max(slices.Max(a), slices.Max(b))
	var sum float64
	n := 0
	for k := range a {
		if max(a[k], b[k]) > top-30 {
			sum += (a[k] - b[k]) * (a[k] - b[k])
			n++
		}
	}
	return math.Sqrt(sum / float64(max(n, 1)))
}

// TestTheKitsSoundApart plays the five drums of each kit of the library
// and of General MIDI through the plugin, and holds every two kits
// apart: some drum of one sounds 1.5 dB or more apart from the other's,
// and 20 dB or more between a chip's kit, the Commodore 64's or the
// NES's, whose drums are their own, and any other kit. Kits that only tune the same drums,
// as the Linn and Hi-NRG kits, sound nearest.
func TestTheKitsSoundApart(t *testing.T) {
	type kit struct {
		name  string
		drums [][]float64
	}
	chip := map[string]bool{"Commodore 64 kit": true, "NES kit": true}
	var ks []kit
	for _, s := range kits() {
		if s.Bank == Songs {
			continue
		}
		k := kit{name: s.Name}
		for _, key := range kitTestNotes {
			k.drums = append(k.drums, fingerprint(kitSound(t, s, key)))
		}
		ks = append(ks, k)
	}
	for i, a := range ks {
		for _, b := range ks[i+1:] {
			most, at := 0.0, 0
			for d := range a.drums {
				if diff := apart(a.drums[d], b.drums[d]); diff > most {
					most, at = diff, kitTestNotes[d]
				}
			}
			t.Logf("%s and %s: %.1f dB apart, on note %d", a.name, b.name, most, at)
			least := 1.5
			if chip[a.name] != chip[b.name] {
				least = 20
			}
			if most < least {
				t.Errorf("%s and %s sound alike: %.1f dB apart at most, on note %d", a.name, b.name, most, at)
			}
		}
	}
}
