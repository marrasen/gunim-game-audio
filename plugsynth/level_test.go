package plugsynth

import (
	"flag"
	"math"
	"math/rand/v2"
	"testing"
)

// allLevels has the level test measure every sound of the library, not a few at random.
var allLevels = flag.Bool("levels.all", false, "the level test measures every sound of the library")

// A few of the library's sounds, picked at random, each playing its level phrase stand within 1 LU of
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
	for _, s := range Sounds() {
		if s.Bank == Library {
			if _, ok := levelKinds[s.Group]; !ok && s.Group != "Leads" && s.Group != "Arps & plucks" &&
				s.Group != "Vocoders & voices" && s.Group != "Effects" && !s.kit() {
				t.Errorf("the library's group %q plays no level phrase of its own: add it to levelKinds", s.Group)
			}
		}
	}
}
