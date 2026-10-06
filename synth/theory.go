package synth

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Notes are MIDI note numbers: 60 is middle C, C4, and 69 is A4, 440 Hz.

// noteHz returns the frequency of note n, which may lie between notes.
func noteHz(n float64) float64 { return 440 * math.Exp2((n-69)/12) }

var pitchNames = [12]string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}

// sharpNames and flatNames spell the pitches as a key with sharps does,
// and a key with flats.
var (
	sharpNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	flatNames  = [12]string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}
)

// NoteName returns note n's name, as C4 or Eb3.
func NoteName(n int) string { return pitchNames[mod(n, 12)] + strconv.Itoa(floorDiv(n, 12)-1) }

// parsePitchClass reads a note's letter and accidentals from the start
// of s, as C, F# or Bb, and returns its pitch class, from 0 for C, and
// how much of s it took.
func parsePitchClass(s string) (pc, n int, ok bool) {
	if s == "" {
		return 0, 0, false
	}
	letters := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	pc, ok = letters[upper(s[0])]
	if !ok {
		return 0, 0, false
	}
	n = 1
	for n < len(s) {
		switch s[n] {
		case '#':
			pc++
		case 'b':
			pc--
		default:
			return mod(pc, 12), n, true
		}
		n++
	}
	return mod(pc, 12), n, true
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// ParseNote reads a note's name, as C4, eb3 or F#5, and returns its
// number.
func ParseNote(s string) (int, error) {
	pc, n, ok := parsePitchClass(s)
	if !ok || n == len(s) {
		return 0, fmt.Errorf("synth: %q is no note, such as C4 or Eb3", s)
	}
	oct, err := strconv.Atoi(s[n:])
	if err != nil {
		return 0, fmt.Errorf("synth: %q is no note, such as C4 or Eb3", s)
	}
	return pc + 12*(oct+1), nil
}

// Scales are the steps of each scale, in semitones up from its root.
var Scales = map[string][]int{
	"major":         {0, 2, 4, 5, 7, 9, 11},
	"minor":         {0, 2, 3, 5, 7, 8, 10},
	"harmonicMinor": {0, 2, 3, 5, 7, 8, 11},
	"dorian":        {0, 2, 3, 5, 7, 9, 10},
	"phrygian":      {0, 1, 3, 5, 7, 8, 10},
	"lydian":        {0, 2, 4, 6, 7, 9, 11},
	"mixolydian":    {0, 2, 4, 5, 7, 9, 10},
	"majorPenta":    {0, 2, 4, 7, 9},
	"minorPenta":    {0, 3, 5, 7, 10},
	"blues":         {0, 3, 5, 6, 7, 10},
}

// key is a song's key: its root and scale.
type key struct {
	root  int
	scale []int
}

func parseKey(root, scale string) (key, error) {
	if root == "" {
		root = "C"
	}
	if scale == "" {
		scale = "major"
	}
	pc, n, ok := parsePitchClass(root)
	if !ok || n != len(root) {
		return key{}, fmt.Errorf("synth: key %q is no note, such as C or F#", root)
	}
	s, ok := Scales[scale]
	if !ok {
		return key{}, fmt.Errorf("synth: no scale %q", scale)
	}
	return key{root: pc, scale: s}, nil
}

// names returns the names k spells its pitches with: sharps for a key
// whose signature has sharps, as E major or C# minor, and flats for
// the rest but C major and A minor, which spell as pitchNames does.
func (k key) names() [12]string {
	major := k.root
	if len(k.scale) > 2 && k.scale[2] == 3 {
		major = mod(k.root+3, 12)
	}
	switch major {
	case 7, 2, 9, 4, 11, 6:
		return sharpNames
	case 5, 10, 3, 8, 1:
		return flatNames
	}
	return pitchNames
}

// degree returns the note of scale degree d, from 0 for the root, in
// octave oct: degree 0 in octave 4 is the root from C4 up.
func (k key) degree(d, oct int) int {
	l := len(k.scale)
	return k.root + 12*(oct+1) + k.scale[mod(d, l)] + 12*floorDiv(d, l)
}

// A Chord is a chord of a progression.
type Chord struct {
	// Name is the chord as written, as Am7, G/B or IV.
	Name string
	// Root is the root's pitch class, from 0 for C, and Tones the notes
	// up from it, in semitones, the first 0.
	Root  int
	Tones []int
	// Bass is the pitch class of the note under the chord, which is the
	// root but for a slash chord.
	Bass int
	// names spell the pitches, as the key does.
	names [12]string
}

// qualities are chord suffixes and their notes up from the root,
// longest first so that m7 is not read as m.
var qualities = []struct {
	suffix string
	tones  []int
}{
	{"m7b5", []int{0, 3, 6, 10}},
	{"maj9", []int{0, 4, 7, 11, 14}},
	{"maj7", []int{0, 4, 7, 11}},
	{"madd9", []int{0, 3, 7, 14}},
	{"7sus4", []int{0, 5, 7, 10}},
	{"dim7", []int{0, 3, 6, 9}},
	{"sus2", []int{0, 2, 7}},
	{"sus4", []int{0, 5, 7}},
	{"add9", []int{0, 4, 7, 14}},
	{"dim", []int{0, 3, 6}},
	{"aug", []int{0, 4, 8}},
	{"sus", []int{0, 5, 7}},
	{"M7", []int{0, 4, 7, 11}},
	{"m9", []int{0, 3, 7, 10, 14}},
	{"m7", []int{0, 3, 7, 10}},
	{"m6", []int{0, 3, 7, 9}},
	{"m", []int{0, 3, 7}},
	{"o", []int{0, 3, 6}},
	{"+", []int{0, 4, 8}},
	{"9", []int{0, 4, 7, 10, 14}},
	{"7", []int{0, 4, 7, 10}},
	{"6", []int{0, 4, 7, 9}},
	{"5", []int{0, 7}},
	{"", []int{0, 4, 7}},
}

// romans are the numerals, longest first.
var romans = []struct {
	s string
	d int
}{{"VII", 6}, {"III", 2}, {"VI", 5}, {"IV", 3}, {"II", 1}, {"V", 4}, {"I", 0}}

// parseChord reads a chord by name, as Am7, F#m, Cmaj7 or G/B, or by its
// numeral in k, as vi, IV, V7, bVII or ii7. A numeral counts degrees of
// k's scale: in A minor VI is F. Upper case is a major chord and lower
// case a minor one, unless a suffix says otherwise.
func parseChord(s string, k key) (Chord, error) {
	c := Chord{Name: s, names: k.names()}
	body, bass, slash := strings.Cut(s, "/")
	if r, rest, ok := parseRoman(body, k); ok {
		c.Root = r.root
		tones, ok := quality(rest)
		if !ok {
			return c, fmt.Errorf("synth: chord %q has a suffix %q to no chord", s, rest)
		}
		switch {
		case rest == "":
			tones = []int{0, 4, 7}
			if r.minor {
				tones = []int{0, 3, 7}
			}
		case rest == "7" && r.minor:
			tones = []int{0, 3, 7, 10}
		case rest == "9" && r.minor:
			tones = []int{0, 3, 7, 10, 14}
		}
		c.Tones = tones
	} else {
		pc, n, ok := parsePitchClass(body)
		if !ok {
			return c, fmt.Errorf("synth: %q is no chord, such as Am7, G/B or IV", s)
		}
		tones, ok := quality(body[n:])
		if !ok {
			return c, fmt.Errorf("synth: chord %q has a suffix %q to no chord", s, body[n:])
		}
		c.Root, c.Tones = pc, tones
	}
	c.Bass = c.Root
	if slash {
		pc, n, ok := parsePitchClass(bass)
		if !ok || n != len(bass) {
			return c, fmt.Errorf("synth: chord %q has a bass %q that is no note", s, bass)
		}
		c.Bass = pc
	}
	return c, nil
}

func quality(suffix string) ([]int, bool) {
	for _, q := range qualities {
		if suffix == q.suffix {
			return q.tones, true
		}
	}
	return nil, false
}

type roman struct {
	root  int
	minor bool
}

// parseRoman reads a numeral and its accidentals from the start of s,
// and returns what is left of it.
func parseRoman(s string, k key) (roman, string, bool) {
	shift := 0
	for s != "" && (s[0] == 'b' || s[0] == '#') && len(s) > 1 && strings.ContainsRune("IViv", rune(s[1])) {
		if s[0] == 'b' {
			shift--
		} else {
			shift++
		}
		s = s[1:]
	}
	for _, r := range romans {
		lower := strings.ToLower(r.s)
		switch {
		case strings.HasPrefix(s, r.s):
			return roman{root: mod(k.degree(r.d, 0)+shift, 12)}, s[len(r.s):], true
		case strings.HasPrefix(s, lower):
			return roman{root: mod(k.degree(r.d, 0)+shift, 12), minor: true}, s[len(lower):], true
		}
	}
	return roman{}, s, false
}

// tone returns chord tone i of c, from the root at its pitch class in
// octave oct up: 0 is the root, 1 the next tone, and past the last they
// go on an octave up; below 0 they go down.
func (c Chord) tone(i, oct int) int {
	l := len(c.Tones)
	return c.Root + 12*(oct+1) + c.Tones[mod(i, l)] + 12*floorDiv(i, l)
}

// voice returns c's notes about centre, each near where prev had its
// notes, so chords played one after another move little, as a pianist's
// hands do. With prev empty it puts the chord's middle near centre.
func (c Chord) voice(centre int, prev []int) []int {
	n := len(c.Tones)
	best, bestCost := []int(nil), math.Inf(1)
	for inv := range n {
		for oct := -2; oct <= 1; oct++ {
			v := make([]int, n)
			for i := range n {
				v[i] = c.tone(inv+i, 4+oct) - 12*(centre/12-5)
			}
			var cost float64
			if len(prev) == n {
				for i := range v {
					cost += math.Abs(float64(v[i] - prev[i]))
				}
				cost += 0.25 * math.Abs(float64(v[0]+v[n-1])/2-float64(centre))
			} else {
				cost = math.Abs(float64(v[0]+v[n-1])/2 - float64(centre))
			}
			if cost < bestCost {
				best, bestCost = v, cost
			}
		}
	}
	return best
}

// Spelled returns the chord by its root's name, as B7 or G/B, whether
// it was written so or by numeral, as V7.
func (c Chord) Spelled() string {
	suffix := ""
	for _, q := range qualities {
		if slicesEqual(q.tones, c.Tones) {
			suffix = q.suffix
			break
		}
	}
	if suffix == "o" {
		suffix = "dim"
	}
	names := c.names
	if names[0] == "" {
		names = pitchNames
	}
	s := names[c.Root] + suffix
	if c.Bass != c.Root {
		s += "/" + names[c.Bass]
	}
	return s
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fits reports whether note n belongs to c.
func (c Chord) fits(n int) bool {
	for _, t := range c.Tones {
		if mod(n-c.Root-t, 12) == 0 {
			return true
		}
	}
	return false
}

// Styles are the kinds of progression GenerateChords writes: how each
// chord leads to the next, by its numeral in the song's key.
var Styles = map[string]Style{
	"pop": {
		Start: map[string]float64{"I": 3, "vi": 2, "IV": 2},
		Next: map[string]map[string]float64{
			"I":   {"IV": 3, "V": 3, "vi": 3, "ii": 1, "iii": 1},
			"ii":  {"V": 4, "IV": 1, "vi": 1},
			"iii": {"vi": 3, "IV": 2},
			"IV":  {"I": 3, "V": 3, "ii": 1, "vi": 1, "iii": 1},
			"V":   {"vi": 3, "I": 3, "IV": 2},
			"vi":  {"IV": 4, "ii": 2, "V": 2, "iii": 1},
		},
		Sevenths: map[string]string{"ii": "ii7", "IV": "IVmaj7", "vi": "vi7", "iii": "iii7"},
	},
	"kpop": {
		Start: map[string]float64{"IV": 4, "vi": 2, "I": 1},
		Next: map[string]map[string]float64{
			"IV":  {"V": 5, "iii": 1, "I": 1},
			"V":   {"iii": 4, "vi": 3, "I": 2},
			"iii": {"vi": 5, "IV": 1},
			"vi":  {"IV": 3, "ii": 2, "V": 2},
			"ii":  {"V": 4, "iii": 1},
			"I":   {"V": 2, "vi": 2, "IV": 2},
		},
		Sevenths: map[string]string{"IV": "IVmaj7", "iii": "iii7", "vi": "vi7", "ii": "ii7", "V": "V7sus4"},
	},
	"epic": {
		Start: map[string]float64{"i": 4, "VI": 1},
		Next: map[string]map[string]float64{
			"i":   {"VI": 3, "iv": 2, "VII": 2, "III": 1},
			"VI":  {"VII": 3, "III": 2, "iv": 1, "i": 1},
			"VII": {"i": 3, "III": 2, "VI": 1},
			"III": {"VII": 2, "VI": 2, "iv": 2},
			"iv":  {"i": 2, "V": 2, "VII": 2, "VI": 1},
			"V":   {"i": 5, "VI": 1},
		},
		Sevenths: map[string]string{"iv": "iv7", "VI": "VImaj7", "V": "V7"},
	},
	"villain": {
		Start: map[string]float64{"i": 5},
		Next: map[string]map[string]float64{
			"i":   {"VI": 2, "iv": 2, "bII": 1, "V": 2},
			"iv":  {"V": 3, "i": 1, "bII": 1},
			"V":   {"i": 5, "VI": 1},
			"VI":  {"V": 2, "bII": 2, "iv": 2, "VII": 1},
			"bII": {"V": 4, "i": 1},
			"VII": {"i": 2, "III": 1},
			"III": {"iv": 2, "VI": 1},
		},
		Sevenths: map[string]string{"V": "V7", "iv": "iv7"},
	},
}

// A Style is how GenerateChords writes progressions.
type Style struct {
	// Start weighs the chords a progression starts on, by numeral.
	Start map[string]float64
	// Next weighs the chords each chord leads to.
	Next map[string]map[string]float64
	// Sevenths are the richer chords some chords become, now and then.
	Sevenths map[string]string
}

// GenerateChords writes a progression of n chords in style, chosen by
// seed: each chord leads to the next as the style likes, the last leads
// back round to the first, and a chord seldom follows itself. sevenths
// is how often a chord becomes its richer form, from 0 to 1.
func GenerateChords(style string, n int, seed uint64, sevenths float64) ([]string, error) {
	st, ok := Styles[style]
	if !ok {
		return nil, fmt.Errorf("synth: no progression style %q", style)
	}
	if n < 1 {
		return nil, nil
	}
	r := newRand(seed ^ 0x5eed)
	out := make([]string, n)
	out[0] = pick(r, st.Start, "")
	for i := 1; i < n; i++ {
		w := st.Next[out[i-1]]
		if i == n-1 && n > 1 {
			// The last chord leads back to the first.
			lead := map[string]float64{}
			for c, x := range w {
				if y := st.Next[c][out[0]]; y > 0 && c != out[0] {
					lead[c] = x * y
				}
			}
			if len(lead) > 0 {
				w = lead
			}
		}
		out[i] = pick(r, w, out[i-1])
	}
	for i, c := range out {
		if s, ok := st.Sevenths[c]; ok && r.float() < sevenths {
			out[i] = s
		}
	}
	return out, nil
}

// pick chooses a key of w by its weight, avoiding not where it can.
func pick(r *rng, w map[string]float64, not string) string {
	keys := make([]string, 0, len(w))
	for k := range w {
		if k != not {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return not
	}
	// Map order is random; the choice must be the seed's alone.
	sortStrings(keys)
	total := 0.0
	for _, k := range keys {
		total += w[k]
	}
	x := r.float() * total
	for _, k := range keys {
		if x -= w[k]; x < 0 {
			return k
		}
	}
	return keys[len(keys)-1]
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// rng is a small, fast random number generator, xorshift64*, for the
// choices a song makes and the noise it plays.
type rng struct{ s uint64 }

func newRand(seed uint64) *rng {
	r := &rng{s: seed*0x9e3779b97f4a7c15 + 0x2545f4914f6cdd1d}
	if r.s == 0 {
		r.s = 1
	}
	r.next()
	return r
}

func (r *rng) next() uint64 {
	r.s ^= r.s >> 12
	r.s ^= r.s << 25
	r.s ^= r.s >> 27
	return r.s * 0x2545f4914f6cdd1d
}

// float returns a number from 0 up to 1.
func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

// intn returns a number from 0 up to n.
func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

// bipolar returns a number from -1 to 1, as noise.
func (r *rng) bipolar() float32 { return float32(int32(r.next()>>32)) / (1 << 31) }

// KeyNote returns the note of scale degree d, from 0 for the root, in
// octave oct of the key root and scale, as a track's degrees play: for a
// tool to name a degree's note.
func KeyNote(root, scale string, d, oct int) (int, error) {
	k, err := parseKey(root, scale)
	if err != nil {
		return 0, err
	}
	return k.degree(d, oct), nil
}
