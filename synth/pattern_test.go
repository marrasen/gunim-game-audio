package synth

import (
	"fmt"
	"strings"
	"testing"
)

// show writes a cycle's events as atom@start, in sixteenths where they
// fall on them.
func show(p *pattern, cycle int) string {
	evs := p.query(cycle, nil)
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, fmt.Sprintf("%s@%g", p.atoms[e.atom], e.at*16))
	}
	return strings.Join(out, " ")
}

func TestPattern(t *testing.T) {
	for _, c := range []struct {
		src   string
		cycle int
		want  string
	}{
		{"bd ~ sn ~", 0, "bd@0 sn@8"},
		{"bd*2 sn", 0, "bd@0 bd@4 sn@8"},
		{"[bd sn]*2", 0, "bd@0 sn@4 bd@8 sn@12"},
		{"<a b c>", 0, "a@0"},
		{"<a b c>", 1, "b@0"},
		{"<a b c>", 5, "c@0"},
		{"<a b>*2", 0, "a@0 b@8"},
		{"x(3,8)", 0, "x@0 x@6 x@12"},
		{"x(5,8)", 0, "x@0 x@4 x@6 x@10 x@12"},
		{"x(3,8,2)", 0, "x@2 x@8 x@12"},
		{"a _ _ b", 0, "a@0 b@12"},
		{"a@3 b", 0, "a@0 b@12"},
		{"a!3 b", 0, "a@0 a@4 a@8 b@12"},
		{"a ! b", 0, "a@0 a@5.333333333333333 b@10.666666666666666"},
		{"[a, b c]", 0, "a@0 b@0 c@8"},
		{"a b . c d e", 0, "a@0 b@4 c@8 d@10.666666666666666 e@13.333333333333332"},
		{"a/2", 0, "a@0"},
		{"a/2", 1, ""},
		{"[a b]/2", 1, "b@0"},
		{"c0 c1' 4# -1", 0, "c0@0 c1'@4 4#@8 -1@12"},
	} {
		p, err := readPattern(c.src)
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if got := show(p, c.cycle); got != c.want {
			t.Errorf("%q, cycle %d: got %q, want %q", c.src, c.cycle, got, c.want)
		}
	}
}

func TestPatternDegrade(t *testing.T) {
	p := mustPattern("hh*16?")
	n := len(p.query(3, nil))
	if n == 0 || n == 16 {
		t.Errorf("hh*16? kept %d of 16", n)
	}
	if again := len(p.query(3, nil)); again != n {
		t.Errorf("a cycle degraded twice kept %d, then %d", n, again)
	}
}

func TestPatternErrors(t *testing.T) {
	for _, src := range []string{"[a b", "<a", "a(3)", "a*", "_ a", "a )"} {
		if _, err := readPattern(src); err == nil {
			t.Errorf("%q parsed", src)
		}
	}
}

func TestChords(t *testing.T) {
	cmaj, _ := parseKey("C", "major")
	amin, _ := parseKey("A", "minor")
	for _, c := range []struct {
		name string
		k    key
		root int
		tone []int
		bass int
	}{
		{"Am7", cmaj, 9, []int{0, 3, 7, 10}, 9},
		{"G/B", cmaj, 7, []int{0, 4, 7}, 11},
		{"vi", cmaj, 9, []int{0, 3, 7}, 9},
		{"IVmaj7", cmaj, 5, []int{0, 4, 7, 11}, 5},
		{"bVII", cmaj, 10, []int{0, 4, 7}, 10},
		{"V7", amin, 4, []int{0, 4, 7, 10}, 4},
		{"VI", amin, 5, []int{0, 4, 7}, 5},
		{"iv7", amin, 2, []int{0, 3, 7, 10}, 2},
		{"bII", amin, 10, []int{0, 4, 7}, 10},
		{"F#m7b5", cmaj, 6, []int{0, 3, 6, 10}, 6},
	} {
		ch, err := parseChord(c.name, c.k)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if ch.Root != c.root || fmt.Sprint(ch.Tones) != fmt.Sprint(c.tone) || ch.Bass != c.bass {
			t.Errorf("%s: got root %d, tones %v, bass %d", c.name, ch.Root, ch.Tones, ch.Bass)
		}
	}
	if _, err := parseChord("Hm", cmaj); err == nil {
		t.Error("Hm parsed")
	}
}

func TestVoiceLeading(t *testing.T) {
	k, _ := parseKey("C", "major")
	var prev []int
	for _, name := range []string{"C", "Am", "F", "G", "C"} {
		ch, _ := parseChord(name, k)
		v := ch.voice(66, prev)
		for _, n := range v {
			if !ch.fits(n) {
				t.Errorf("%s voiced %v, with %s", name, v, NoteName(n))
			}
		}
		if prev != nil {
			moved := 0
			for i := range v {
				moved += max(v[i]-prev[i], prev[i]-v[i])
			}
			if moved > 7 {
				t.Errorf("%s voiced %v after %v: the voices moved %d semitones", name, v, prev, moved)
			}
		}
		prev = v
	}
}

func TestGenerateChords(t *testing.T) {
	k, _ := parseKey("A", "minor")
	for style := range Styles {
		for seed := range uint64(20) {
			got, err := GenerateChords(style, 4, seed, 0.3)
			if err != nil || len(got) != 4 {
				t.Fatalf("%s, seed %d: %v, %v", style, seed, got, err)
			}
			for i, c := range got {
				if _, err := parseChord(c, k); err != nil {
					t.Errorf("%s wrote %q: %v", style, c, err)
				}
				if i > 0 && c == got[i-1] {
					t.Errorf("%s wrote %v: a chord follows itself", style, got)
				}
			}
			again, _ := GenerateChords(style, 4, seed, 0.3)
			if fmt.Sprint(again) != fmt.Sprint(got) {
				t.Errorf("%s, seed %d wrote %v, then %v", style, seed, got, again)
			}
		}
	}
}

func TestNotes(t *testing.T) {
	for name, want := range map[string]int{"C4": 60, "A4": 69, "Eb3": 51, "F#5": 78, "c-1": 0} {
		if got, err := ParseNote(name); err != nil || got != want {
			t.Errorf("%s: got %d, %v; want %d", name, got, err, want)
		}
	}
	if NoteName(61) != "C#4" {
		t.Errorf("61 is %s", NoteName(61))
	}
}

func TestTranspose(t *testing.T) {
	s := &Song{Key: "C", Chords: []string{"C Am7", "F/A", "vi", "_"},
		Tracks: []*Track{{Pattern: "C4 [Eb5 c0] 2 bd"}},
		Keypad: &Keypad{Notes: []string{"C5", "D5"}},
		Stings: map[string]*Sting{"win": {Key: "D", Chords: []string{"Bb"}}},
	}
	s.Transpose(2)
	got := strings.TrimSpace(fmt.Sprintln(s.Key, s.Chords, s.Tracks[0].Pattern, s.Keypad.Notes, s.Stings["win"].Key, s.Stings["win"].Chords))
	want := "D [D Bm7 G/B vi _] D4 [F5 c0] 2 bd [D5 E5] E [C]"
	if got != want {
		t.Errorf("transposed to %s, want %s", got, want)
	}
	if k := (&Song{Key: "A", Scale: "harmonicMinor"}).KeyName(); k != "A harmonic minor" {
		t.Errorf("key name %q", k)
	}
}

func TestSpelled(t *testing.T) {
	k, _ := parseKey("E", "major")
	for name, want := range map[string]string{"V": "B", "IVmaj7": "Amaj7", "iii7": "G#m7", "Isus4": "Esus4", "G/B": "G/B", "viio": "D#dim"} {
		c, err := parseChord(name, k)
		if err != nil || c.Spelled() != want {
			t.Errorf("%s: spelled %q, %v; want %q", name, c.Spelled(), err, want)
		}
	}
}

func TestStepsWriteBackAsTheyRead(t *testing.T) {
	for _, src := range []string{
		"bd ~ sn ~", "[c0 c1]*2 c2", "<0 2 4>", "bd(3,8)", "bd(3,8,2)", "hh*16?", "hh*8?0.3",
		"0 2@2 4", "[bd, hh*4]", "<[a b] c>/2", "[sh*8, ~ cp ~ cp, ~ ~ ~ ~ ~ ~ ~ [~ rim]]",
	} {
		s, err := ParsePattern(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		back := s.String()
		for c := range 4 {
			want := show(mustPattern(src), c)
			if got := show(mustPattern(back), c); got != want {
				t.Errorf("%q wrote back as %q, which plays %q in cycle %d, not %q", src, back, got, c, want)
			}
		}
	}
	if got := PatternCycles("<a b c> <d e>"); got != 6 {
		t.Errorf("<a b c> <d e> comes round in %d cycles, want 6", got)
	}
}
