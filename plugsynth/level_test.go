package plugsynth

import (
	"flag"
	"github.com/marrasen/gunim-game-audio/soundfont"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// allLevels has the level test measure every sound of the library, not a few at random.
var allLevels = flag.Bool("levels.all", false, "the level test measures every sound of the library")

// A few of the library's sounds, kits too, picked at random, each playing its level phrase stand within 1 LU of
// LevelTarget, or, a sharp sound, short of it with its true peak at LevelPeak. None peaks past LevelPeak. A sound
// that breaks the rule was edited without its Gain measured again: run `go run ./cmd/levels -write`.
func TestTheLibrarysSoundsStandEquallyLoud(t *testing.T) {
	t.Parallel()
	var sounds []Sound
	for _, s := range Sounds() {
		if Levelled(s) {
			sounds = append(sounds, s)
		}
	}
	if !*allLevels {
		seed := rand.Uint64()
		t.Logf("seed %d", seed)
		r := rand.New(rand.NewPCG(seed, 0))
		r.Shuffle(len(sounds), func(i, j int) { sounds[i], sounds[j] = sounds[j], sounds[i] })
		sounds = sounds[:4]
	}
	for _, s := range sounds {
		t.Run(s.Name, func(t *testing.T) {
			t.Parallel()
			lufs, peak, err := MeasureLevel(s, 48000)
			if err != nil {
				t.Fatal(err)
			}
			capped := math.Abs(peak-LevelPeak) <= 0.3 && lufs < LevelTarget
			if math.Abs(lufs-LevelTarget) > 1 && !capped {
				t.Errorf("%s plays its %s phrase at %.1f LUFS, peaking at %.1f dBFS, not at %.0f LUFS: run go run ./cmd/levels -write",
					s.Name, LevelKind(s), lufs, peak, LevelTarget)
			}
			if peak > LevelPeak+0.3 {
				t.Errorf("%s peaks at %.1f dBFS true peak, past %.1f", s.Name, peak, LevelPeak)
			}
		})
	}
}

// Every sound the level phrases name plays one of the library's.
func TestTheLevelKindsNameTheLibrarysSounds(t *testing.T) {
	t.Parallel()
	for name := range levelOwn {
		s, ok := Named(name)
		if !ok || !Levelled(s) {
			t.Errorf("levelOwn names %q, no sound of the library", name)
		}
	}
	for _, name := range slices.Concat(slices.Collect(maps.Keys(levelRecorded)), slices.Collect(maps.Keys(levelShift)), slices.Collect(maps.Keys(levelKits))) {
		if !slices.ContainsFunc(soundfont.Instruments, func(b soundfont.Bank) bool { return strings.HasPrefix(name, b.Name) }) {
			t.Errorf("%q names no recorded instrument", name)
		}
	}
	for _, s := range Sounds() {
		if s.Bank == Library {
			if _, ok := levelKinds[s.Group]; !ok && s.Group != "Leads" && s.Group != "Arps & plucks" &&
				s.Group != "Vocoders & voices" && s.Group != "Effects" && !s.kit() {
				t.Errorf("the library's group %q plays no level phrase of its own: add it to levelKinds", s.Group)
			}
		}
	}
}

// A kit with a kick, a snare and a closed hat plays them as a backbeat. A kit of other keys plays through those it has.
func TestAKitsLevelPhraseFitsItsKeys(t *testing.T) {
	count := func(notes []levelNote, pitch float64) int {
		n := 0
		for _, x := range notes {
			if x.pitch == pitch {
				n++
			}
		}
		return n
	}
	kit := func(name string) Sound {
		s, ok := Named(name)
		if !ok {
			t.Fatalf("no sound named %s", name)
		}
		return s
	}
	notes := levelDrumNotes(kit("TR-808"))
	if count(notes, 36) != 8 || count(notes, 38) != 8 || count(notes, 42) != 32 {
		t.Errorf("the backbeat plays %d kicks, %d snares and %d hats", count(notes, 36), count(notes, 38), count(notes, 42))
	}
	// The test bank plays every key but these.
	levelKits["Test Walk"] = [3]float64{-1, -2, -3}
	t.Cleanup(func() { delete(levelKits, "Test Walk") })
	useTestFont(t, Font{Name: "test-walk", Title: "Test Walk", Group: "Drums", Kit: true})
	walk, ok := Find(Recorded, "Drums", "Test Walk")
	if !ok {
		t.Fatal("no recorded kit")
	}
	notes = levelDrumNotes(walk)
	if len(notes) != 32 {
		t.Fatalf("the walk plays %d notes, not 32", len(notes))
	}
	f, _ := fontNamed("test-walk")
	p := walk.Patch()
	for _, n := range notes {
		if len(f.Font.Regions(p.Bank, p.Program, int(n.pitch), 100)) == 0 {
			t.Errorf("the walk plays key %v, which the kit does not have", n.pitch)
		}
	}
}
