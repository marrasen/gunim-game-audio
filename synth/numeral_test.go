package synth

import (
	"slices"
	"testing"
)

// TestEveryChordReadsBackFromItsNumeral: every chord, on every root,
// over every bass, written by numeral in every key and scale, reads
// back to the same root, tones and bass, and so does its spelled name.
func TestEveryChordReadsBackFromItsNumeral(t *testing.T) {
	for _, q := range qualities {
		for root := range 12 {
			for bass := range 12 {
				name := pitchNames[root] + q.suffix
				if bass != root {
					name += "/" + pitchNames[bass]
				}
				for keyRoot := range 12 {
					for scale := range Scales {
						want, err := ParseChord(name, pitchNames[keyRoot], scale)
						if err != nil {
							t.Fatalf("%s: %v", name, err)
						}
						n, err := want.Numeral(pitchNames[keyRoot], scale)
						if err != nil {
							t.Fatalf("%s in %s %s: %v", name, pitchNames[keyRoot], scale, err)
						}
						if !IsNumeral(n) {
							t.Fatalf("%s in %s %s is %q, which reads as no numeral", name, pitchNames[keyRoot], scale, n)
						}
						for _, back := range []string{n, want.Spelled()} {
							got, err := ParseChord(back, pitchNames[keyRoot], scale)
							if err != nil || got.Root != want.Root || got.Bass != want.Bass || !slices.Equal(got.Tones, want.Tones) {
								t.Fatalf("%s in %s %s, written %q, reads %d %v /%d (%v), want %d %v /%d",
									name, pitchNames[keyRoot], scale, back, got.Root, got.Tones, got.Bass, err, want.Root, want.Tones, want.Bass)
							}
						}
					}
				}
			}
		}
	}
}

func TestNumeralsReadAsMusiciansWriteThem(t *testing.T) {
	for _, c := range []struct{ name, root, scale, want string }{
		{"Am", "C", "major", "vi"},
		{"Am7", "C", "major", "vi7"},
		{"Fmaj7", "C", "major", "IVmaj7"},
		{"Bm7b5", "C", "major", "viim7b5"},
		{"Bb", "C", "major", "bVII"},
		{"C/E", "C", "major", "I/3"},
		{"Am/C", "C", "major", "vi/b3"},
		{"C7/Bb", "C", "major", "I7/b7"},
		{"Am/G", "C", "major", "vi/b7"},
		{"E", "A", "harmonicMinor", "V"},
		{"F#dim", "C", "major", "bvdim"},
		{"Eb", "D", "minorPenta", "#I"},
	} {
		ch, err := ParseChord(c.name, c.root, c.scale)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := ch.Numeral(c.root, c.scale); got != c.want {
			t.Errorf("%s in %s %s is %q, want %q", c.name, c.root, c.scale, got, c.want)
		}
	}
}

func TestASlashChordsBassReadsAsAnInterval(t *testing.T) {
	for _, c := range []struct{ name, root, scale, want string }{
		{"V/3", "C", "major", "G/B"},
		{"V/3", "D", "major", "A/C#"},
		{"I/b7", "C", "major", "C/Bb"},
		{"vi/b3", "C", "major", "Am/C"},
		{"IV/5", "Eb", "major", "Ab/Eb"},
		{"bbVII", "C", "major", "A"},
		{"C/9", "C", "major", "C/D"},
		{"C/#11", "C", "major", "C/F#"},
	} {
		ch, err := ParseChord(c.name, c.root, c.scale)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := ch.Spelled(); got != c.want {
			t.Errorf("%s in %s %s is %s, want %s", c.name, c.root, c.scale, got, c.want)
		}
	}
	for _, bad := range []string{"C/8", "C/b9b", "I/x3", "C/H"} {
		if _, err := ParseChord(bad, "C", "major"); err == nil {
			t.Errorf("%s read as a chord", bad)
		}
	}
}

func TestIsNumeral(t *testing.T) {
	for name, want := range map[string]bool{
		"vi": true, "bVII": true, "#ivdim": true, "I/3": true, "bbII": true,
		"Am": false, "Bb": false, "bm": false, "G/B": false, "b": false, "": false,
	} {
		if got := IsNumeral(name); got != want {
			t.Errorf("IsNumeral(%q) is %v", name, got)
		}
	}
}

func TestTransposeMovesABassByIntervalWithItsRoot(t *testing.T) {
	if got := transposeChords("C/b7 G/B vi/b3", 2); got != "D/b7 A/C# vi/b3" {
		t.Fatalf("transposed by 2: %q", got)
	}
}
