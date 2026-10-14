package synth

import (
	"slices"
	"strings"
)

// vowelCycles is how many cycles of a track's vowels Vowels looks
// through for the first that sings any.
const vowelCycles = 64

// Vowels returns the vowels the song sings on the patch named patch,
// note by note, for an [Instrument] to sing them in turn as
// [Patch.Vowels]: the vowels of the first track that plays the patch
// and changes its vowel, in the first cycle that holds any, as its
// vowel parameter writes them; a run of them that repeats is given
// once. It returns "" where no track changes the patch's vowel.
func (s *Song) Vowels(patch string) string {
	for _, t := range s.Tracks {
		src, ok := t.Params["vowel"]
		if t.Patch != patch || !ok {
			continue
		}
		pat, err := readPattern(src)
		if err != nil {
			continue
		}
		var evs []pev
		for cycle := range vowelCycles {
			evs = pat.query(cycle, evs[:0])
			var vs []string
			for _, e := range evs {
				if a := pat.atoms[e.atom]; len(a) == 1 && strings.Contains(vowelOrder, a) {
					vs = append(vs, a)
				}
			}
			if len(vs) > 0 {
				return strings.Join(period(vs), " ")
			}
		}
	}
	return ""
}

// period returns the shortest run of vs that, repeated, makes vs.
func period(vs []string) []string {
	for n := 1; n < len(vs); n++ {
		if len(vs)%n != 0 {
			continue
		}
		rep := true
		for i := n; i < len(vs) && rep; i += n {
			rep = slices.Equal(vs[i:i+n], vs[:n])
		}
		if rep {
			return vs[:n]
		}
	}
	return vs
}
