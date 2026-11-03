package plugsynth

import (
	"fmt"
	"math"
	"slices"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/plug"
)

// The library's sounds stand equally loud: each patch's Gain is set so it plays its level phrase at LevelTarget,
// or its true peak at LevelPeak where that comes first.
// The phrases are Hej Jätte!'s level phrases, so the two synths' sounds stand as loud as each other. Every phrase
// plays at 120 BPM over Am, F, C and G, a bar each, every note at velocity 0.8 and held for 90% of its step, then
// rings for 2 s. `go run ./cmd/levels` measures the sounds, and with -write sets their Gains.

// LevelTarget is the loudness every sound of the library is set to, in LUFS, playing its level phrase.
const LevelTarget = -13.0

// LevelPeak is the highest true peak, in dBFS, a sound of the library is set to. A sharp sound, a pluck or a stab,
// reaches it before LevelTarget, and stands quieter. The synth has no limiter: this is Hej Jätte!'s rule, whose
// limiter at -1 dB is given at most 3 dB past where a sound's peak reaches -1.5 dBFS.
const LevelPeak = 1.5

// The rules every level phrase keeps.
const (
	levelBPM      = 120
	levelVelocity = 0.8
	levelGate     = 0.9
	levelTail     = 2
	levelBeats    = 16
)

// The kinds of level phrase.
const (
	// LevelHeld holds the chord a bar: three tones and the root an octave up.
	LevelHeld = "held"
	// LevelStabs strikes that chord on each offbeat eighth.
	LevelStabs = "stabs"
	// LevelLow plays eighths on the chord's root around A1, with its octave and fifth.
	LevelLow = "low"
	// LevelLine plays eighths through the chord's tones an octave up.
	LevelLine = "line"
	// LevelKeys strikes the chord on beats one and three, under a line of quarters.
	LevelKeys = "keys"
)

// levelChords are the phrases' chords, Am, F, C and G, their tones around A3, and levelRoots their roots around
// A1.
var (
	levelChords = [][3]float64{{57, 60, 64}, {57, 60, 65}, {55, 60, 64}, {55, 59, 62}}
	levelRoots  = []float64{33, 29, 36, 31}
)

// levelKinds are the kinds of phrase the library's categories play, and levelOwn the kinds of the sounds that
// play another than their category's.
var (
	levelKinds = map[string]string{
		"Bass": LevelLow, "Pads & strings": LevelHeld, "Brass & stabs": LevelHeld, "Keys & bells": LevelKeys,
		// General MIDI's families, and the groups of the recorded instruments.
		"Piano": LevelKeys, "Organ": LevelKeys, "Strings": LevelHeld, "Ensemble": LevelHeld, "Brass": LevelHeld,
		"Synth pad": LevelHeld,
	}
	levelOwn = map[string]string{
		"Orchestra hit": LevelStabs, "Chord stab": LevelStabs,
		"Tubular bells": LevelLine, "Glockenspiel": LevelLine,
		"Sung choir": LevelHeld, "Vocoder choir": LevelHeld,
		"Wind sweep": LevelHeld, "Machine drone": LevelHeld,
	}
	// levelRecorded are the kinds of the recorded instruments that play another than their group's.
	levelRecorded = map[string]string{"Concert Harp": LevelLine}
	// levelShift moves a sound's phrase by that many semitones, into its range.
	levelShift = map[string]float64{"Timpani": -24}
)

// Levelled says whether the sound is one of the library's sounds the levels set: any but a drum kit.
func Levelled(s Sound) bool { return s.Bank == Library && !s.kit() }

// LevelKind returns the kind of level phrase the sound is measured by.
func LevelKind(s Sound) string {
	if k, ok := levelOwn[s.Name]; ok {
		return k
	}
	if k, ok := levelRecorded[s.Name]; ok {
		return k
	}
	if k, ok := levelKinds[s.Group]; ok {
		return k
	}
	return LevelLine
}

// levelNote is a note of a level phrase: when it starts and how long it lasts, in beats, and its pitch.
type levelNote struct{ at, length, pitch float64 }

// levelPhrase returns the notes of the level phrase of a kind.
func levelPhrase(kind string) []levelNote {
	var out []levelNote
	note := func(at, step, pitch float64) {
		out = append(out, levelNote{at: at, length: step * levelGate, pitch: pitch})
	}
	for b, c := range levelChords {
		bar := float64(4 * b)
		tones := []float64{c[0], c[1], c[2], c[0] + 12}
		chord := func(at, step float64) {
			for _, p := range tones {
				note(at, step, p)
			}
		}
		switch kind {
		case LevelHeld:
			chord(bar, 4)
		case LevelStabs:
			for k := range 4 {
				chord(bar+float64(k)+0.5, 0.5)
			}
		case LevelLow:
			for k, up := range []float64{0, 0, 12, 0, 7, 0, 12, 0} {
				note(bar+float64(k)/2, 0.5, levelRoots[b]+up)
			}
		case LevelKeys:
			chord(bar, 2)
			chord(bar+2, 2)
			for k, i := range []int{2, 1, 3, 1} {
				note(bar+float64(k), 1, tones[i]+12)
			}
		default:
			for k, i := range []int{0, 1, 2, 3, 2, 1, 2, 1} {
				note(bar+float64(k)/2, 0.5, tones[i]+12)
			}
		}
	}
	return out
}

// MeasureLevel renders the sound playing its level phrase at rate and returns its loudness, in LUFS, and its
// true peak, in dBFS.
func MeasureLevel(s Sound, rate int) (lufs, peak float64, err error) {
	pl := Descriptor.New(nil)
	defer pl.Close()
	if err := pl.SetState(s.State()); err != nil {
		return 0, 0, fmt.Errorf("%s: %w", s.Name, err)
	}
	p := pl.Processor()
	const block = 256
	p.Prepare(plug.Setup{Rate: float64(rate), MaxBlock: block, Channels: 2, Offline: true})
	var changes []plug.Change
	for _, prm := range Descriptor.Params {
		if prm.Flags&plug.ReadOnly == 0 {
			changes = append(changes, plug.Change{ID: prm.ID, Value: prm.Default})
		}
	}
	notes := levelPhrase(LevelKind(s))
	for i := range notes {
		notes[i].pitch += levelShift[s.Name]
	}
	frameOf := func(beats float64) int { return int(math.Round(beats * 60 / levelBPM * float64(rate))) }
	ahead := int(lookAhead.Seconds() * float64(rate))
	total := frameOf(levelBeats) + levelTail*rate
	told := make([]bool, len(notes))
	l, r := make([]float32, block), make([]float32, block)
	frames := make([]float32, 2*block)
	loud := audio.NewLoudnessMeter(rate)
	var tp audio.TruePeakMeter
	for done := 0; done < total; done += block {
		m := min(block, total-done)
		b := &plug.Block{Out: [][]float32{l[:m], r[:m]}, Frames: m, Changes: changes}
		changes = nil
		for i, n := range notes {
			on, off := frameOf(n.at), frameOf(n.at+n.length)
			id := int32(i + 1)
			if f := on - done; !told[i] && f >= m && f < ahead {
				told[i] = true
				b.Upcoming = append(b.Upcoming, plug.Upcoming{Kind: plug.Coming, Frame: f, Note: id, Pitch: n.pitch,
					Velocity: levelVelocity, Turn: int64(i + 1)})
			}
			if f := on - done; f >= 0 && f < m {
				told[i] = true
				b.Events = append(b.Events, plug.Event{Frame: f, Kind: plug.NoteOn, Note: id, Pitch: n.pitch,
					Velocity: levelVelocity, Turn: int64(i + 1)})
			}
			if f := off - done; f >= 0 && f < m {
				b.Events = append(b.Events, plug.Event{Frame: f, Kind: plug.NoteOff, Note: id})
			}
		}
		slices.SortStableFunc(b.Events, func(x, y plug.Event) int { return x.Frame - y.Frame })
		b.Position = plug.Position{Frame: int64(done), Song: int64(done), Playing: true, Tempo: levelBPM,
			Beat: float64(done) / float64(rate) * levelBPM / 60}
		p.Process(b)
		for i := range m {
			frames[2*i], frames[2*i+1] = l[i], r[i]
		}
		loud.Write(frames[:2*m])
		tp.Write(frames[:2*m])
	}
	lufs, ok := loud.Integrated()
	if !ok {
		return 0, 0, fmt.Errorf("%s plays nothing", s.Name)
	}
	return lufs, 20 * math.Log10(tp.Peak()), nil
}
