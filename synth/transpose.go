package synth

import (
	"regexp"
	"strings"
)

// Transpose moves the song by n semitones: its key, the chords it names,
// as Am7 or G/B, and the notes its patterns and keypad name, as C4. Its
// chords by numeral, as vi, and its notes by degree move with the key
// already.
func (s *Song) Transpose(n int) {
	if n == 0 {
		return
	}
	s.Key = transposeName(s.Key, n)
	if s.Key == "" {
		s.Key = pitchNames[mod(n, 12)]
	}
	for i, c := range s.Chords {
		s.Chords[i] = transposeChords(c, n)
	}
	for _, t := range s.Tracks {
		t.Pattern = transposeNotes(t.Pattern, n)
	}
	if kp := s.Keypad; kp != nil {
		for i, note := range kp.Notes {
			kp.Notes[i] = transposeNotes(note, n)
		}
	}
	for _, st := range s.Stings {
		if st.Key != "" {
			st.Key = transposeName(st.Key, n)
		}
		for i, c := range st.Chords {
			st.Chords[i] = transposeChords(c, n)
		}
		for _, t := range st.Tracks {
			t.Pattern = transposeNotes(t.Pattern, n)
		}
	}
}

// transposeChords moves each chord named in s, split by spaces, by n.
func transposeChords(s string, n int) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "_" {
			continue
		}
		if _, _, ok := parseRoman(w, key{scale: Scales["major"]}); ok {
			continue
		}
		body, bass, slash := strings.Cut(w, "/")
		body = transposeName(body, n)
		if slash {
			body += "/" + transposeName(bass, n)
		}
		words[i] = body
	}
	return strings.Join(words, " ")
}

// transposeName moves the note at the start of s, as C, F# or Bb, by n,
// keeping what follows it.
func transposeName(s string, n int) string {
	pc, k, ok := parsePitchClass(s)
	if !ok {
		return s
	}
	return pitchNames[mod(pc+n, 12)] + s[k:]
}

// noteWord finds the notes a pattern names, as C4 or Eb5.
var noteWord = regexp.MustCompile(`\b[A-G][#b]*-?\d\b`)

// transposeNotes moves each note named in s by n.
func transposeNotes(s string, n int) string {
	return noteWord.ReplaceAllStringFunc(s, func(w string) string {
		note, err := ParseNote(w)
		if err != nil {
			return w
		}
		return NoteName(note + n)
	})
}

// KeyName returns the song's key as a person says it, as C major or
// A minor.
func (s *Song) KeyName() string {
	k, sc := s.Key, s.Scale
	if k == "" {
		k = "C"
	}
	if sc == "" {
		sc = "major"
	}
	// harmonicMinor reads as harmonic minor.
	var b strings.Builder
	for i, r := range sc {
		if r >= 'A' && r <= 'Z' && i > 0 {
			b.WriteByte(' ')
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return k + " " + b.String()
}
