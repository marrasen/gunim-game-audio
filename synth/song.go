package synth

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/marrasen/gunim/audio/band"
)

// A Song is music made in code, which plays as a game's music: a
// tempo, a key and a chord progression, patches to play, and tracks
// that play them in patterns over the chords. It is a [band.Song], and
// its player is a [band.Tiered], a [band.Triggered] and a
// [band.Watcher], as well as a [*Player], which plays notes on a keypad,
// plays stings and takes edits as it plays.
//
// A Song is plain data, read from JSON as a song.json holds it. A song
// handed to [Player.SetSong] or played belongs to the player; change a
// [Song.Clone] of it instead.
type Song struct {
	Title, Artist string
	// Category is the group the song shows in, in a list of songs, as
	// Boss fights or Calm rooms; see music.Category.
	Category string
	// BPM is the tempo, BeatsPerBar 4 by default, and PhraseBars how
	// many bars a phrase lasts, 8 by default: tiers change, and parts
	// come and go, where phrases start.
	BPM         float64
	BeatsPerBar int
	PhraseBars  int
	// Key is the key's root, as C or F#, and Scale its scale, major by
	// default; see Scales.
	Key, Scale string
	// Chords are the progression, each chord ChordBars bars long, 1 by
	// default, as Am F C G or vi IV I V; a _ holds the chord before
	// another ChordBars. Where Chords is empty, Generate writes them.
	Chords    []string
	ChordBars float64
	Generate  *Generate
	// Mode is tiers, where a program sets the tier and the tracks up to
	// it play, or wander, where the tracks come and go by themselves.
	// Start is the tier a tiers song starts at, 1 by default.
	Mode  string
	Start int
	// Swing delays each second sixteenth by as much of a sixteenth, from
	// 0 to 0.5, for a shuffle.
	Swing float64
	// Keypad plays notes for a program's keys; see Player.Key.
	Keypad *Keypad
	// Clap names the sound a dancer claps its hands to, so it claps in
	// time with the song: a clap, or any track's notes, as a bell's;
	// see Player.Claps.
	Clap *ClapCue `json:",omitempty"`
	// Patches are the instruments, by name.
	Patches map[string]*Patch
	Tracks  []*Track
	// Stings are short endings by name, as a victory's; see
	// Player.Sting.
	Stings map[string]*Sting
	Mix    Mix
}

// A ClapCue names the sound a dancer claps its hands to: the notes of
// track Track, or where Drum is set, that drum's hits on it.
type ClapCue struct {
	Track string
	Drum  string `json:",omitempty"`
}

// Generate writes a song's progression; see GenerateChords.
type Generate struct {
	// Style is pop, kpop, epic or villain; see Styles.
	Style string
	// Chords is how many chords there are, 4 by default.
	Chords int
	Seed   uint64
	// Sevenths is how often a chord becomes richer, from 0 to 1.
	Sevenths float64
	// Evolve, where set, writes a new progression every Evolve phrases,
	// so the song wanders on through new chords.
	Evolve int
}

// A Track plays a patch in a pattern over the chords.
type Track struct {
	Name string
	// Tier is the tier the track plays from, in a tiers song. Core
	// keeps it playing always in a wander song, where the rest come and
	// go.
	Tier int
	Core bool
	// Patch names the patch it plays.
	Patch string
	// Pattern is what it plays, in mini-notation, over Bars bars, 1 by
	// default. A drums track plays drums by name, as bd ~ sn ~. Any
	// other plays notes:
	//
	//	0 2 4     degrees of the song's scale, 0 its root, in Octave
	//	4# 6b     a degree a semitone up or down
	//	c0 c1 c2  the chord's tones, c0 its root, in Octave
	//	ch        the whole chord, voiced near Octave's middle
	//	b         the chord's bass note
	//	x         the next note of the arpeggio, where Arp is set
	//	C4 Eb5    notes by name
	//
	// and each ' after a note lifts it an octave.
	Pattern string
	Bars    int
	// Octave is the octave the track plays in, 4 by default: degree 0
	// in octave 4 is the root from C4 up.
	Octave int
	// Arp arpeggiates the chord, x by x: up, down, updown, downup,
	// converge or random, over ArpOctaves octaves, 1 by default.
	Arp        string
	ArpOctaves int
	// Legato is how much of its step a note is held, 0.9 by default.
	Legato float64
	// Melody, where set, writes the track's notes in place of Pattern.
	Melody *Melody
	// Params change each note, as TidalCycles' controls do: patterns
	// of values by name, each note taking the value playing as it
	// starts. vel and pan, from -1 to 1; cutoff, in hertz; res; legato;
	// octave; vowel, a e i o or u; and tune, in semitones. A value may
	// be a signal, sine, tri, saw, square or rand, from 0 to 1, or from
	// lo to hi as sine:lo:hi, over cycles as sine:lo:hi:cycles.
	Params map[string]string
	// Gain is the track's level in decibels; Pan its place between the
	// speakers, from -1 to 1; Reverb and Delay how much it sends to the
	// song's reverb and delay, from 0 to 1; and Duck how far the song's
	// duck track turns it down as it hits, from 0 to 1, as a sidechain
	// pumps a dance track.
	Gain, Pan, Reverb, Delay, Duck float64
	// Gated is how much it sends to the song's gated reverb, from 0 to
	// 1; each of its hits opens the gate.
	Gated float64 `json:",omitempty"`
	// HPF and LPF cut it below and above them, in hertz; Shape drives
	// it, from 0 to 1; Crush takes it to that many bits; Coarse holds
	// each sample that many frames; and Chorus thickens it, from 0 to 1.
	HPF, LPF, Shape, Crush float64
	Coarse                 int
	Chorus                 float64
	// Distort distorts it, from 0 to 1, as DistortType says: fuzz, the
	// default, clipped hard; amp, a guitar's amplifier and its speaker's
	// cabinet; or fold, a wavefolder's metallic folds.
	Distort     float64 `json:",omitempty"`
	DistortType string  `json:",omitempty"`
	// Ring ring-modulates it, from 0 to 1, with a sine at RingHz, 440 by
	// default: the clangorous, metallic voice of a ring modulator, as
	// industrial records put on their snares and their voices.
	Ring   float64 `json:",omitempty"`
	RingHz float64 `json:",omitempty"`
	// Smash mixes under it, from 0 to 1, a copy of it crushed by a
	// compressor with every ratio's button in and driven, as a room's
	// drums are smashed so they pump.
	Smash float64 `json:",omitempty"`
	// ChorusType is the chorus's kind: soft, the default, two copies
	// swaying slowly; juno1, juno2 or juno12, a Roland Juno-60's chorus
	// I, II, or both buttons down; or ensemble, a string machine's.
	ChorusType string `json:",omitempty"`
	// Human moves each note a little in time and level, from 0 to 1, as
	// a player's hands do.
	Human float64
	// Mute silences the track, and Solo silences every track but those
	// soloed, as a mixing desk's buttons do.
	Mute, Solo bool
	// Color is the track's colour in a tool, as #ff6b9d.
	Color string
}

// Melody writes a track's notes: a motif over each chord, its strong
// beats on the chord's tones and its steps between them on the
// scale's, repeated and varied as a hook is.
type Melody struct {
	Seed uint64
	// Density is how busy it is, from 0 to 1, 0.5 by default.
	Density float64
	// Low and High bound it, as scale degrees in the track's octave.
	Low, High int
	// Penta keeps it to the key's major pentatonic, leaving out the
	// notes that clash.
	Penta bool
	// Evolve, where set, writes a new melody every Evolve phrases.
	Evolve int
}

// Keypad plays a note for each of a program's digit keys, in key, over
// the song; see [Player.Key].
type Keypad struct {
	// Patch names the patch it plays.
	Patch string
	// Notes are the notes of the keys 1 to 9, and then 0, as C5. By
	// default they climb the major pentatonic of the song's key from
	// its root in octave 5.
	Notes []string
	// Gain, Pan, Reverb and Delay are as a track's.
	Gain, Pan, Reverb, Delay float64
}

// A Sting is a short ending, as a victory fanfare: tracks over chords of
// its own for a few bars, in place of the song.
type Sting struct {
	// Bars is how long it lasts, and Chords its progression, a chord a
	// bar, in the song's key unless Key and Scale say another.
	Bars       int
	Chords     []string
	Key, Scale string
	Tracks     []*Track
	// Then says what follows it: stop, the default, for silence, or
	// resume, for the song again from its first tier.
	Then string
}

// Mix is how a song's tracks come together.
type Mix struct {
	// Reverb is the room every track sends to.
	Reverb Reverb
	// Delay is the echo every track sends to.
	Delay Echo
	// Duck names the track whose hits turn the others down, as far as
	// each one's Duck says, as a kick drum pumps a dance track.
	Duck string
	// Gain is the whole mix's level in decibels.
	Gain float64
	// Threshold and Ratio set the compressor that glues the mix, -10 dB
	// and 2 by default.
	Threshold, Ratio float64
	// Gated is a second room, its sound cut off short by a gate that the
	// hits of the tracks sending to it open, as the 1980s gated a
	// snare's room; nil for none.
	Gated *Gated `json:",omitempty"`
	// MonoBass makes the mix mono below it, in hertz, as a record's
	// cutting engineer does, so the needle tracks the bass.
	MonoBass float64 `json:",omitempty"`
	// Air lifts the mix above 10 kHz by that many decibels, as the
	// mastering of the 1980s brightened a record.
	Air float64 `json:",omitempty"`
	// Tape saturates the mix as a tape machine does, from 0 to 1: the
	// quiet as it was, the peaks rounded off.
	Tape float64 `json:",omitempty"`
	// Tweak turns the whole song's sound by a few broad knobs.
	Tweak Tweak `json:",omitzero"`
	// Transitions names a drums patch to mark tier changes with: a riser
	// in the bars before the tier climbs, an impact as it lands, and a
	// down as it falls. Lift is how many bars the riser takes, 1 by
	// default; Gain its level in decibels.
	Transitions *Transitions
}

// Reverb shapes a song's reverb: Size from 0.3 to 1.5, Decay in seconds,
// Tone from 0, dark, to 1, bright, and PreDelay in seconds.
type Reverb struct {
	Size, Decay, Tone, PreDelay float64
}

// Tweak turns a whole song's sound by a few broad knobs, as a
// listener's tone controls do, over the mix as it is made. Each is 0
// for the song as mixed.
type Tweak struct {
	// Tone tilts the sound round 800 Hz, from -1, dark, to 1, bright:
	// the top up and the bottom down by up to 6 dB, or the other way.
	Tone float64 `json:",omitzero"`
	// Bass lifts the bass below about 120 Hz by up to 9 dB, or cuts it,
	// from -1 to 1.
	Bass float64 `json:",omitzero"`
	// Space is how much of the rooms and echoes is heard, from -1, none,
	// to 1, three times as much.
	Space float64 `json:",omitzero"`
	// Punch squeezes the mix, from 0 to 1: the compressor's threshold
	// down by up to 18 dB and its ratio up to 8, its level made up.
	Punch float64 `json:",omitzero"`
	// Width narrows the sound to mono at -1, or widens it to twice its
	// sides at 1.
	Width float64 `json:",omitzero"`
	// Drive saturates the mix, from 0 to 1, as a desk pushed too hard.
	Drive float64 `json:",omitzero"`
	// LoFi makes it an old radio's, from 0 to 1: fewer bits, a lower
	// sample rate, and its top and bottom cut.
	LoFi float64 `json:",omitzero"`
}

// Gated shapes a song's gated reverb: Size and Tone as a Reverb's, 1.2
// and 0.6 by default, and Hold how long, in seconds, its gate stays open
// after a hit, 0.3 by default, before it shuts in a few milliseconds.
type Gated struct {
	Size, Tone, Hold float64
}

// Echo times a song's delay: Beats between echoes, 0.75 by default,
// Feedback, how loud each is after the one before, and Tone, from dark
// to bright.
type Echo struct {
	Beats, Feedback, Tone float64
}

// Transitions mark a tiers song's tier changes.
type Transitions struct {
	Patch string
	Lift  int
	Gain  float64
	// Reverb is how much they send to the reverb.
	Reverb float64
}

// Info implements [band.Song].
func (s *Song) Info() band.Info { return band.Info{Title: s.Title, Artist: s.Artist, BPM: s.BPM} }

// Play implements [band.Song]. A song that does not compile plays
// silence; [Song.Check] says why.
func (s *Song) Play(seed uint64) band.Player {
	return NewPlayer(s, seed)
}

// Check reports what keeps the song from playing, if anything.
func (s *Song) Check() error {
	_, err := compile(s)
	return err
}

// Clone returns a copy of the song to change.
func (s *Song) Clone() *Song {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	c := new(Song)
	if err := json.Unmarshal(b, c); err != nil {
		panic(err)
	}
	return c
}

// Parse reads a song from JSON, as a song.json holds it, and checks it.
func Parse(b []byte) (*Song, error) {
	s := new(Song)
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	err := s.Check()
	return s, err
}

// compiled is a song made ready to play: its names read, its patterns
// parsed, its chords laid out.
type compiled struct {
	s          *Song
	key        key
	bpm        float64
	beats      int
	phraseBars int
	// fpb is how many frames a bar lasts.
	fpb       float64
	chordBars float64
	// progs are the progressions, each its chords in order; one unless
	// the song evolves.
	progs   [][]Chord
	evolve  int
	patches map[string]*patch
	tracks  []*ctrack
	byName  map[string]int
	tiers   int
	start   int
	wander  bool
	keypad  *ckeypad
	stings  map[string]*csting
	duck    int
	trans   *ctrans
	swing   float64
	// clap is the track a dancer claps to, -1 for none, and clapDrum
	// its drum, or "" for each of its notes.
	clap     int
	clapDrum string
}

// ctrack is a track made ready to play.
type ctrack struct {
	t      *Track
	patch  *patch
	pat    *pattern
	acts   []action
	params []cparam
	bars   int
	oct    int
	arp    int
	arpOct int
	legato float64
	gain   float32
	pan    float32
	// mel are the bars of a written melody, a slice a bar of the
	// progression, a set for each of its progressions and evolutions.
	mel [][][]mnote
}

// mnote is a note of a written melody: where it starts in its bar, how
// long it lasts, in bars, and its note.
type mnote struct {
	at, dur float64
	pitch   int
	vel     float32
}

type ckeypad struct {
	t     *ctrack
	notes [10]int
}

type csting struct {
	s      *Sting
	key    key
	chords []Chord
	tracks []*ctrack
	resume bool
}

type ctrans struct {
	patch  *patch
	lift   int
	gain   float32
	reverb float32
	riser  drum
	down   drum
	boom   drum
	crash  drum
}

// The kinds of action an atom asks a track for.
const (
	actDegree = iota
	actTone
	actChord
	actBass
	actArp
	actNote
	actDrum
)

// action is what an atom asks a track to play.
type action struct {
	kind int
	// n is the degree, chord tone or note; acc a degree's sharp or
	// flat; and oct the octaves the atom lifts it by.
	n, acc, oct int
	drum        drum
	// tone is the chord tone a pitched drum plays, -1 for its own.
	tone int
}

// The parameters a track's notes take.
const (
	parVel = iota
	parPan
	parCutoff
	parRes
	parLegato
	parOctave
	parVowel
	parTune
	parCount
)

var paramNames = map[string]int{
	"vel": parVel, "gain": parVel, "velocity": parVel, "pan": parPan, "cutoff": parCutoff, "lpf": parCutoff,
	"res": parRes, "resonance": parRes, "legato": parLegato, "octave": parOctave, "vowel": parVowel, "tune": parTune,
}

type cparam struct {
	which int
	pat   *pattern
	vals  []pval
}

// pval is a parameter's value: a number, a vowel, or a signal from lo
// to hi over period cycles.
type pval struct {
	num            float64
	sig            int
	lo, hi, period float64
}

// The signals a parameter may follow, sigNone for a plain value.
const (
	sigNone = iota
	sigSine
	sigTri
	sigSaw
	sigSquare
	sigRand
)

func compile(s *Song) (*compiled, error) {
	if s == nil {
		return nil, errors.New("synth: no song")
	}
	c := &compiled{s: s, bpm: s.BPM, beats: s.BeatsPerBar, phraseBars: s.PhraseBars, chordBars: s.ChordBars,
		byName: map[string]int{}, duck: -1, swing: min(max(s.Swing, 0), 0.5)}
	if c.bpm <= 0 {
		return nil, fmt.Errorf("synth: song %q has no tempo", s.Title)
	}
	if c.beats == 0 {
		c.beats = 4
	}
	if c.phraseBars == 0 {
		c.phraseBars = 8
	}
	if c.chordBars <= 0 {
		c.chordBars = 1
	}
	c.fpb = rate * 60 * float64(c.beats) / c.bpm
	var err error
	if c.key, err = parseKey(s.Key, s.Scale); err != nil {
		return nil, err
	}
	if err := c.compileChords(); err != nil {
		return nil, err
	}
	c.patches = map[string]*patch{}
	for name, p := range s.Patches {
		if p == nil {
			return nil, fmt.Errorf("synth: patch %s is empty", name)
		}
		cp, err := p.compile(name)
		if err != nil {
			return nil, err
		}
		c.patches[name] = cp
	}
	switch s.Mode {
	case "", "tiers":
	case "wander":
		c.wander = true
	default:
		return nil, fmt.Errorf("synth: song %q has mode %q, not tiers or wander", s.Title, s.Mode)
	}
	for i, t := range s.Tracks {
		if t == nil || t.Name == "" {
			return nil, fmt.Errorf("synth: track %d has no name", i+1)
		}
		if _, ok := c.byName[t.Name]; ok {
			return nil, fmt.Errorf("synth: two tracks are named %s", t.Name)
		}
		ct, err := c.compileTrack(t, c.key, c.progs, false)
		if err != nil {
			return nil, err
		}
		if !c.wander {
			if t.Tier < 1 {
				return nil, fmt.Errorf("synth: track %s has no tier", t.Name)
			}
			c.tiers = max(c.tiers, t.Tier)
		}
		c.byName[t.Name] = len(c.tracks)
		c.tracks = append(c.tracks, ct)
	}
	if len(c.tracks) == 0 {
		return nil, fmt.Errorf("synth: song %q has no tracks", s.Title)
	}
	c.start = min(max(s.Start, 1), max(c.tiers, 1))
	if s.Mix.Duck != "" {
		i, ok := c.byName[s.Mix.Duck]
		if !ok {
			return nil, fmt.Errorf("synth: the mix ducks to track %s, which there is none of", s.Mix.Duck)
		}
		c.duck = i
	}
	c.clap = -1
	if cl := s.Clap; cl != nil {
		i, ok := c.byName[cl.Track]
		if !ok {
			return nil, fmt.Errorf("synth: a dancer claps to track %s, which there is none of", cl.Track)
		}
		if c.tracks[i].mel != nil {
			return nil, fmt.Errorf("synth: a dancer claps to track %s, a written melody; clap to a track of a pattern", cl.Track)
		}
		c.clap, c.clapDrum = i, cl.Drum
	}
	if s.Keypad != nil {
		if err := c.compileKeypad(s.Keypad); err != nil {
			return nil, err
		}
	}
	if tr := s.Mix.Transitions; tr != nil {
		p, ok := c.patches[tr.Patch]
		if !ok || p.kind != kindDrums {
			return nil, fmt.Errorf("synth: the transitions play patch %q, which is no drums patch of the song", tr.Patch)
		}
		c.trans = &ctrans{patch: p, lift: max(tr.Lift, 1), gain: float32(dbGain(tr.Gain)), reverb: float32(tr.Reverb),
			riser: p.kit["riser"], down: p.kit["down"], boom: p.kit["boom"], crash: p.kit["cr"]}
		c.trans.lift = min(c.trans.lift, c.phraseBars)
	}
	c.stings = map[string]*csting{}
	for name, st := range s.Stings {
		cs, err := c.compileSting(name, st)
		if err != nil {
			return nil, err
		}
		c.stings[name] = cs
	}
	return c, nil
}

// compileChords lays out the progression, or writes it.
func (c *compiled) compileChords() error {
	s := c.s
	names := [][]string{s.Chords}
	if len(s.Chords) == 0 {
		g := s.Generate
		if g == nil {
			g = &Generate{Style: "pop"}
			if c.key.scale[2] == 3 {
				g.Style = "epic"
			}
		}
		n := g.Chords
		if n == 0 {
			n = 4
		}
		versions := 1
		if g.Evolve > 0 {
			c.evolve = g.Evolve
			versions = 8
		}
		names = names[:0]
		for v := range versions {
			ch, err := GenerateChords(g.Style, n, g.Seed+uint64(v)*1013, g.Sevenths)
			if err != nil {
				return err
			}
			names = append(names, ch)
		}
	}
	for _, list := range names {
		prog, err := chordList(list, c.key)
		if err != nil {
			return err
		}
		c.progs = append(c.progs, prog)
	}
	return nil
}

// chordList reads a progression, a _ holding the chord before.
func chordList(names []string, k key) ([]Chord, error) {
	var out []Chord
	for _, n := range names {
		for _, w := range strings.Fields(n) {
			if w == "_" {
				if len(out) == 0 {
					return nil, errors.New("synth: a progression starts with _")
				}
				out = append(out, out[len(out)-1])
				continue
			}
			ch, err := readChord(w, k)
			if err != nil {
				return nil, err
			}
			out = append(out, ch)
		}
	}
	if len(out) == 0 {
		out = []Chord{{Name: "I", Root: k.root, Tones: []int{0, 4, 7}, Bass: k.root}}
	}
	return out, nil
}

// prog returns the progression of song bar b.
func (c *compiled) prog(bar int) []Chord {
	if c.evolve == 0 || len(c.progs) == 1 {
		return c.progs[0]
	}
	return c.progs[(bar/(c.phraseBars*c.evolve))%len(c.progs)]
}

// chordAt returns the chord at pos, in bars of the song, and its index
// in the progression.
func (c *compiled) chordAt(pos float64) (ch Chord, index int) {
	prog := c.prog(int(pos))
	i := mod(int(math.Floor(pos/c.chordBars+1e-9)), len(prog))
	return prog[i], i
}

// compileTrack readies t, in key k over progs; a sting's track, in
// sting, may last any number of bars.
func (c *compiled) compileTrack(t *Track, k key, progs [][]Chord, sting bool) (*ctrack, error) {
	ct := &ctrack{t: t, bars: t.Bars, oct: t.Octave, legato: t.Legato, pan: float32(t.Pan), gain: float32(dbGain(t.Gain)), arpOct: max(t.ArpOctaves, 1)}
	p, ok := c.patches[t.Patch]
	if !ok {
		return nil, fmt.Errorf("synth: track %s plays patch %q, which the song has none of", t.Name, t.Patch)
	}
	ct.patch = p
	if ct.bars == 0 {
		ct.bars = 1
	}
	if !sting && c.phraseBars%ct.bars != 0 && ct.bars%c.phraseBars != 0 {
		return nil, fmt.Errorf("synth: track %s lasts %d bars, which does not divide the song's %d-bar phrase", t.Name, ct.bars, c.phraseBars)
	}
	if ct.oct == 0 {
		ct.oct = 4
	}
	if ct.legato == 0 {
		ct.legato = 0.9
	}
	switch t.Arp {
	case "", "up":
		ct.arp = arpUp
	case "down":
		ct.arp = arpDown
	case "updown":
		ct.arp = arpUpDown
	case "downup":
		ct.arp = arpDownUp
	case "converge":
		ct.arp = arpConverge
	case "random":
		ct.arp = arpRandom
	default:
		return nil, fmt.Errorf("synth: track %s arpeggiates %q, not up, down, updown, downup, converge or random", t.Name, t.Arp)
	}
	if _, ok := distortKinds[t.DistortType]; !ok {
		return nil, fmt.Errorf("synth: track %s distorts as %q, not fuzz, amp or fold", t.Name, t.DistortType)
	}
	if _, ok := chorusKinds[t.ChorusType]; !ok {
		return nil, fmt.Errorf("synth: track %s has chorus %q, not soft, juno1, juno2, juno12 or ensemble", t.Name, t.ChorusType)
	}
	src := t.Pattern
	if t.Melody != nil {
		if p.kind == kindDrums {
			return nil, fmt.Errorf("synth: track %s writes a melody for drums", t.Name)
		}
		ct.mel = writeMelodies(t.Melody, k, progs, c.chordBars, ct.oct)
		src = "~"
	}
	if strings.TrimSpace(src) == "" {
		src = "~"
	}
	pat, err := readPattern(src)
	if err != nil {
		return nil, fmt.Errorf("synth: track %s: %w", t.Name, err)
	}
	ct.pat = pat
	for _, a := range pat.atoms {
		act, err := parseAction(a, p)
		if err != nil {
			return nil, fmt.Errorf("synth: track %s: %w", t.Name, err)
		}
		ct.acts = append(ct.acts, act)
	}
	for _, name := range sortedKeys(t.Params) {
		which, ok := paramNames[name]
		if !ok {
			return nil, fmt.Errorf("synth: track %s has a parameter %q, which is none of vel, pan, cutoff, res, legato, octave, vowel or tune", t.Name, name)
		}
		pp, err := readPattern(t.Params[name])
		if err != nil {
			return nil, fmt.Errorf("synth: track %s, %s: %w", t.Name, name, err)
		}
		cp := cparam{which: which, pat: pp}
		for _, a := range pp.atoms {
			v, err := parseValue(a, which)
			if err != nil {
				return nil, fmt.Errorf("synth: track %s, %s: %w", t.Name, name, err)
			}
			cp.vals = append(cp.vals, v)
		}
		ct.params = append(ct.params, cp)
	}
	return ct, nil
}

// The ways of an arpeggio.
const (
	arpUp = iota
	arpDown
	arpUpDown
	arpDownUp
	arpConverge
	arpRandom
)

// parseAction reads what atom a asks a track of patch p to play.
func parseAction(a string, p *patch) (action, error) {
	act := action{tone: -1}
	if p.kind == kindDrums {
		name, tone, hasTone := strings.Cut(a, ":")
		d, ok := p.kit[name]
		if !ok {
			return act, fmt.Errorf("no drum %q in the kit", name)
		}
		act.kind, act.drum = actDrum, d
		if hasTone {
			n, err := strconv.Atoi(tone)
			if err != nil {
				return act, fmt.Errorf("drum %q has a chord tone %q that is no number", a, tone)
			}
			act.tone = n
		}
		return act, nil
	}
	body := strings.TrimRight(a, "'")
	act.oct = len(a) - len(body)
	switch {
	case body == "ch" || body == "chord":
		act.kind = actChord
	case body == "b":
		act.kind = actBass
	case body == "x":
		act.kind = actArp
	case len(body) > 1 && body[0] == 'c' && isNumber(body[1:]):
		act.kind = actTone
		act.n, _ = strconv.Atoi(body[1:])
	case body != "" && body[0] >= 'A' && body[0] <= 'G':
		n, err := ParseNote(body)
		if err != nil {
			return act, err
		}
		act.kind, act.n = actNote, n
	default:
		num := strings.TrimRight(body, "#b")
		for _, r := range body[len(num):] {
			if r == '#' {
				act.acc++
			} else {
				act.acc--
			}
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			return act, fmt.Errorf("%q is no note: a degree as 2, a chord tone as c1, ch, b, x or a note as C4", a)
		}
		act.kind, act.n = actDegree, n
	}
	return act, nil
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// parseValue reads a parameter's value.
func parseValue(a string, which int) (pval, error) {
	if which == parVowel {
		if _, ok := formants[a[0]]; !ok || len(a) != 1 {
			return pval{}, fmt.Errorf("%q is no vowel, a, e, i, o or u", a)
		}
		return pval{num: float64(a[0])}, nil
	}
	parts := strings.Split(a, ":")
	sigs := map[string]int{"sine": sigSine, "tri": sigTri, "saw": sigSaw, "square": sigSquare, "rand": sigRand}
	if sig, ok := sigs[parts[0]]; ok {
		v := pval{sig: sig, lo: 0, hi: 1, period: 1}
		nums := []*float64{&v.lo, &v.hi, &v.period}
		for i, s := range parts[1:] {
			if i >= len(nums) {
				return v, fmt.Errorf("signal %q has too many values: %s:lo:hi:cycles", a, parts[0])
			}
			x, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return v, fmt.Errorf("signal %q has %q, which is no number", a, s)
			}
			*nums[i] = x
		}
		if v.period <= 0 {
			v.period = 1
		}
		return v, nil
	}
	x, err := strconv.ParseFloat(a, 64)
	if err != nil {
		return pval{}, fmt.Errorf("%q is no number, nor a signal such as sine:0:1", a)
	}
	return pval{num: x}, nil
}

// at returns the value at pos, in cycles of the track.
func (v pval) at(pos float64) float64 {
	if v.sig == sigNone {
		return v.num
	}
	x := pos/v.period - math.Floor(pos/v.period)
	var s float64
	switch v.sig {
	case sigSine:
		s = 0.5 + 0.5*math.Sin(2*math.Pi*x)
	case sigTri:
		s = 1 - math.Abs(2*x-1)
	case sigSaw:
		s = x
	case sigSquare:
		if x < 0.5 {
			s = 1
		}
	case sigRand:
		s = chance(0x51ab, int(math.Floor(pos*64)), 0)
	}
	return v.lo + (v.hi-v.lo)*s
}

func (c *compiled) compileKeypad(kp *Keypad) error {
	t := &Track{Name: "keypad", Patch: kp.Patch, Gain: kp.Gain, Pan: kp.Pan, Reverb: kp.Reverb, Delay: kp.Delay, Tier: 1}
	ct, err := c.compileTrack(t, c.key, c.progs, false)
	if err != nil {
		return fmt.Errorf("synth: keypad: %w", err)
	}
	if ct.patch.kind == kindDrums {
		return errors.New("synth: the keypad plays a drums patch")
	}
	k := &ckeypad{t: ct}
	if len(kp.Notes) == 0 {
		penta := key{root: c.key.root, scale: Scales["majorPenta"]}
		if c.key.scale[2] == 3 {
			// A minor key's pentatonic is the relative major's.
			penta = key{root: c.key.root, scale: Scales["minorPenta"]}
		}
		for d := range 10 {
			// Keys 1 to 9 climb from the root; 0 tops them.
			i := d - 1
			if d == 0 {
				i = 9
			}
			k.notes[d] = penta.degree(i, 5)
		}
	} else {
		if len(kp.Notes) != 10 {
			return fmt.Errorf("synth: the keypad has %d notes, and needs 10: the keys 1 to 9 and 0", len(kp.Notes))
		}
		for i, n := range kp.Notes {
			note, err := ParseNote(n)
			if err != nil {
				return err
			}
			k.notes[(i+1)%10] = note
		}
	}
	c.keypad = k
	return nil
}

func (c *compiled) compileSting(name string, st *Sting) (*csting, error) {
	if st == nil || st.Bars < 1 {
		return nil, fmt.Errorf("synth: sting %s lasts no bars", name)
	}
	cs := &csting{s: st, key: c.key}
	switch st.Then {
	case "", "stop":
	case "resume":
		cs.resume = true
	default:
		return nil, fmt.Errorf("synth: sting %s is followed by %q, not stop or resume", name, st.Then)
	}
	if st.Key != "" || st.Scale != "" {
		k, err := parseKey(st.Key, st.Scale)
		if err != nil {
			return nil, err
		}
		cs.key = k
	}
	chords, err := chordList(st.Chords, cs.key)
	if err != nil {
		return nil, fmt.Errorf("synth: sting %s: %w", name, err)
	}
	cs.chords = chords
	for _, t := range st.Tracks {
		ct, err := c.compileTrack(t, cs.key, [][]Chord{chords}, true)
		if err != nil {
			return nil, fmt.Errorf("synth: sting %s: %w", name, err)
		}
		cs.tracks = append(cs.tracks, ct)
	}
	return cs, nil
}
