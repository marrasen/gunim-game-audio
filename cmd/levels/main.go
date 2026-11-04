// Command levels measures how loud each sound of the patch library plays its level phrase, as
// [plugsynth.MeasureLevel] does, and with -write sets each patch's Gain so it plays at
// [plugsynth.LevelTarget], or its true peak at [plugsynth.LevelPeak] where that comes first. With -fonts it does the
// same for the fetched SoundFont banks, whose Levels are in soundfont/banks.go. Run it from the module's root.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/marrasen/gunim-game-audio/plugsynth"
)

// rate is the rate the sounds are measured at.
const rate = 48000

// result is a sound measured: its loudness and true peak, or why it could not be.
type result struct {
	lufs, peak float64
	err        error
}

func main() {
	write := flag.Bool("write", false, "set each patch's Gain in patches/ so it plays at the target")
	fontsToo := flag.Bool("fonts", false, "measure the fetched SoundFont banks instead, each at 0 dB, and print the Level each wants; with -write, set it")
	verbose := flag.Bool("v", false, "with -fonts, print every sound measured")
	flag.Parse()
	if *fontsToo {
		fonts(*verbose, *write)
		return
	}
	files, err := patchFiles("patches")
	if err != nil {
		log.Fatal(err)
	}
	var sounds []plugsynth.Sound
	for _, s := range plugsynth.Sounds() {
		if plugsynth.Levelled(s) {
			sounds = append(sounds, s)
		}
	}
	res := measureSounds(sounds)
	fmt.Printf("Target %.1f LUFS. Each sound plays its level phrase.\n\n", plugsynth.LevelTarget)
	fmt.Printf("%-22s %-6s %8s %8s %8s %8s\n", "sound", "kind", "now", "peak", "gain", "set")
	for i, s := range sounds {
		r := res[i]
		if r.err != nil {
			log.Fatal(r.err)
		}
		g := s.Patch().Gain
		if g == 0 {
			g = 1
		}
		to := round3(g * math.Pow(10, min(plugsynth.LevelTarget-r.lufs, plugsynth.LevelPeak-r.peak)/20))
		fmt.Printf("%-22s %-6s %8.1f %8.1f %8.3g %8.3g\n", s.Name, plugsynth.LevelKind(s), r.lufs, r.peak, g, to)
		if !*write || math.Abs(to-g) < 0.006*g {
			continue
		}
		file, ok := files[s.Name]
		if !ok {
			log.Fatalf("%s has no file in patches/library.json", s.Name)
		}
		if err := setGain(file, to); err != nil {
			log.Fatal(err)
		}
	}
}

// round3 rounds g to three significant digits.
func round3(g float64) float64 {
	scale := math.Pow(10, 2-math.Floor(math.Log10(g)))
	return math.Round(g*scale) / scale
}

// patchFiles returns the file of each preset of dir/library.json, by its name.
func patchFiles(dir string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "library.json"))
	if err != nil {
		return nil, err
	}
	var lib struct {
		Categories []struct {
			Presets []struct{ Name, File string }
		}
	}
	if err := json.Unmarshal(b, &lib); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range lib.Categories {
		for _, p := range c.Presets {
			out[p.Name] = filepath.Join(dir, filepath.FromSlash(p.File))
		}
	}
	return out, nil
}

// topGain finds a patch's own Gain, one tab in, as the files are written.
var topGain = regexp.MustCompile(`(?m)^\t"Gain": [0-9.eE+-]+`)

// setGain writes g as the Gain of the patch in file, in place of its own or after its last field, leaving the
// rest of the file as it is.
func setGain(file string, g float64) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	field := "\t\"Gain\": " + strconv.FormatFloat(g, 'g', -1, 64)
	s := string(b)
	if topGain.MatchString(s) {
		s = topGain.ReplaceAllLiteralString(s, field)
	} else {
		end := regexp.MustCompile(`\n}\s*$`).FindStringIndex(s)
		if end == nil {
			return fmt.Errorf("%s does not end as a patch does", file)
		}
		s = s[:end[0]] + ",\n" + field + "\n}\n"
	}
	return os.WriteFile(file, []byte(s), 0o644)
}
