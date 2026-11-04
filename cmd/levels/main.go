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
	write := flag.Bool("write", false, "set each patch's Gain in patches/, or each drum's in a kit, so it plays at the target")
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
		// A kit's hits are not turned by the patch's Gain: each drum has its own. Its gain here is what they are all
		// turned by, from where they stand.
		g := s.Patch().Gain
		if g == 0 || plugsynth.LevelKind(s) == plugsynth.LevelDrums {
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
		if plugsynth.LevelKind(s) == plugsynth.LevelDrums {
			err = turnKit(file, to)
		} else {
			err = setGain(file, to)
		}
		if err != nil {
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
// drumEntry matches one drum of a patch's Kit, as a patch file is laid out: its name and its lines.
var drumEntry = regexp.MustCompile(`(?m)^\t\t"\w+": \{\n((?:\t\t\t.*\n)+)\t\t\},?\n`)

// emptyDrum matches a drum of a patch's Kit that sets nothing.
var emptyDrum = regexp.MustCompile(`(?m)^(\t\t"\w+": )\{\}(,?)$`)

// drumGain matches a drum's own Gain.
var drumGain = regexp.MustCompile(`(?m)^\t\t\t"Gain": ([0-9.eE+-]+)(,?)$`)

// turnKit multiplies the Gain of every drum of the kit in file by by.
func turnKit(file string, by float64) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	out, err := turnedKit(b, by)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return os.WriteFile(file, out, 0o644)
}

// turnedKit returns the patch file src with the Gain of every drum of its kit multiplied by by.
func turnedKit(src []byte, by float64) ([]byte, error) {
	var p struct{ Kit map[string]json.RawMessage }
	if err := json.Unmarshal(src, &p); err != nil {
		return nil, err
	}
	n := 0
	out := drumEntry.ReplaceAllFunc(src, func(m []byte) []byte {
		n++
		if g := drumGain.FindSubmatch(m); g != nil {
			old, err := strconv.ParseFloat(string(g[1]), 64)
			if err != nil {
				return m
			}
			field := fmt.Appendf(nil, "\t\t\t\"Gain\": %s$2", strconv.FormatFloat(round3(old*by), 'g', -1, 64))
			return drumGain.ReplaceAll(m, field)
		}
		field := fmt.Appendf(nil, ",\n\t\t\t\"Gain\": %s\n\t\t}", strconv.FormatFloat(round3(by), 'g', -1, 64))
		return regexp.MustCompile(`\n\t\t\}`).ReplaceAllLiteral(m, field)
	})
	out = emptyDrum.ReplaceAllFunc(out, func(m []byte) []byte {
		n++
		field := fmt.Appendf(nil, "$1{\n\t\t\t\"Gain\": %s\n\t\t}$2", strconv.FormatFloat(round3(by), 'g', -1, 64))
		return emptyDrum.ReplaceAll(m, field)
	})
	if n != len(p.Kit) {
		return nil, fmt.Errorf("found %d drums of %d", n, len(p.Kit))
	}
	if !json.Valid(out) {
		return nil, fmt.Errorf("turning the kit broke the file")
	}
	return out, nil
}

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
