package main

import (
	"errors"
	"fmt"
	"go/format"
	"log"
	"math"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"sync"

	music "github.com/marrasen/gunim-game-audio"
	"github.com/marrasen/gunim-game-audio/plugsynth"
	"github.com/marrasen/gunim-game-audio/soundfont"
	"github.com/marrasen/gunim-game-audio/soundfont/fontcache"
)

// bankLevel is some sounds of a bank measured: their loudness at 0 dB, their median, the loudest true peak, and the
// Level they want.
type bankLevel struct {
	bank         soundfont.Bank
	name         string
	median, peak float64
	lowest, most float64
	n            int
	level        float64
}

// measureSounds measures each sound, on as many goroutines as there are processors.
func measureSounds(sounds []plugsynth.Sound) []result {
	res := make([]result, len(sounds))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, s := range sounds {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			r := &res[i]
			r.lufs, r.peak, r.err = plugsynth.MeasureLevel(s, rate)
		})
	}
	wg.Wait()
	return res
}

// measureBank plays every sound of bank b at 0 dB and returns how loud its instruments stand, and its kits, where it
// has them. It reports no levels where b is not fetched.
func measureBank(b soundfont.Bank, verbose bool) []bankLevel {
	path, ok := fontcache.Fetched(b)
	if !ok {
		log.Printf("%s is not fetched: skipped", b.Name)
		return nil
	}
	f, err := soundfont.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	// A sound reads its samples on a goroutine as it starts; read them all first, so the render hears them.
	for _, p := range f.Presets {
		f.Preload(p.Bank, p.Program)
	}
	name := b.Key()
	if b.Group == "" {
		name = music.SoundFont
	}
	plugsynth.UseFont(plugsynth.Font{Name: name, Title: b.Name, Group: b.Group, Kit: b.Kit, Font: f})
	defer plugsynth.DropFont(name)
	var sounds, kits []plugsynth.Sound
	for _, s := range plugsynth.Sounds() {
		switch {
		case b.Group == "" && s.Bank == b.Name && s.Group == plugsynth.GMDrums:
			kits = append(kits, s)
		case b.Group == "" && s.Bank == b.Name, b.Group != "" && s.Bank == plugsynth.Recorded:
			// One bank is used at a time, so the recorded instruments are b's.
			sounds = append(sounds, s)
		}
	}
	if len(sounds) == 0 {
		log.Fatalf("%s has no sounds to measure", b.Name)
	}
	out := []bankLevel{measured(b, b.Name, sounds, verbose)}
	if len(kits) > 0 {
		out = append(out, measured(b, b.Name+" kits", kits, verbose))
	}
	return out
}

// measured measures the sounds of bank b and the Level they want.
func measured(b soundfont.Bank, name string, sounds []plugsynth.Sound, verbose bool) bankLevel {
	bl := bankLevel{bank: b, name: name, peak: math.Inf(-1), lowest: math.Inf(1), most: math.Inf(-1)}
	var ls, wants []float64
	for i, r := range measureSounds(sounds) {
		if errors.Is(r.err, plugsynth.ErrSilent) {
			log.Print(r.err)
			continue
		}
		if r.err != nil {
			log.Fatal(r.err)
		}
		if verbose {
			fmt.Printf("  %-28s %-24s %-6s %7.1f %7.1f\n", sounds[i].Name, sounds[i].Group, plugsynth.LevelKind(sounds[i]),
				r.lufs, r.peak)
		}
		ls = append(ls, r.lufs)
		wants = append(wants, min(plugsynth.LevelTarget-r.lufs, plugsynth.LevelPeak-r.peak))
		bl.peak = max(bl.peak, r.peak)
		bl.lowest, bl.most = min(bl.lowest, r.lufs), max(bl.most, r.lufs)
	}
	if len(ls) == 0 {
		log.Fatalf("%s plays nothing", name)
	}
	slices.Sort(ls)
	slices.Sort(wants)
	bl.n = len(ls)
	bl.median = ls[len(ls)/2]
	switch {
	case name != b.Name:
		// A bank's kits are alike but for a few odd ones, so the middle kit sets their Level, to its target or its peak.
		bl.level = math.Floor(2*wants[len(wants)/2]) / 2
	default:
		bl.level = wantLevel(b, bl.median, bl.peak)
	}
	return bl
}

// wantLevel is the Level bank b wants for its instruments, to half a decibel. A General MIDI bank holds every kind of
// sound, some far from the others, so only its median goes to the target. A recorded instrument is one sound, and its
// peak may come first.
func wantLevel(b soundfont.Bank, median, peak float64) float64 {
	want := math.Round(2*(plugsynth.LevelTarget-median)) / 2
	if b.Group != "" {
		// Rounded down, so the peak stays under LevelPeak.
		want = min(want, math.Floor(2*(plugsynth.LevelPeak-peak))/2)
	}
	return want
}

// setLevels returns the source of banks.go with field, Level or KitLevel, of each bank named in levels set. It formats
// what it returns. A bank's Level comes right after its name, and its KitLevel after that.
func setLevels(src []byte, field string, levels map[string]float64) ([]byte, error) {
	before := ""
	if field == "KitLevel" {
		before = `(?:[ \t]*Level:.*\n)?`
	}
	for name, level := range levels {
		re := regexp.MustCompile(`(?m)(^[ \t]*Name:\s*"` + regexp.QuoteMeta(name) + `",.*\n` + before + `)([ \t]*` + field + `:.*\n)?`)
		if len(re.FindAll(src, -1)) != 1 {
			return nil, fmt.Errorf("no one bank named %q", name)
		}
		line := fmt.Sprintf("%s: %s,\n", field, strconv.FormatFloat(level, 'f', -1, 64))
		src = re.ReplaceAllFunc(src, func(m []byte) []byte {
			return append(re.ReplaceAll(m, []byte("$1")), line...)
		})
	}
	return format.Source(src)
}

// banksFile is where the banks' Levels are, from the module's root.
const banksFile = "soundfont/banks.go"

// fonts measures every bank and prints the Levels each wants. With write it sets them in banksFile.
func fonts(verbose, write bool) {
	fmt.Printf("Target %.1f LUFS, peaks to %.1f dBFS. Each sound plays its level phrase at 0 dB.\n\n",
		plugsynth.LevelTarget, plugsynth.LevelPeak)
	var out []bankLevel
	for _, b := range slices.Concat(soundfont.Banks, soundfont.Instruments) {
		if verbose {
			fmt.Println(b.Name)
		}
		out = append(out, measureBank(b, verbose)...)
	}
	fmt.Printf("\n%-26s %3s %8s %8s %8s %8s %8s %8s\n", "bank", "n", "median", "low", "high", "peak", "now", "set")
	levels, kits := map[string]float64{}, map[string]float64{}
	for _, bl := range out {
		now := bl.bank.Level
		if bl.name != bl.bank.Name {
			now = bl.bank.KitLevel
			kits[bl.bank.Name] = bl.level
		} else {
			levels[bl.bank.Name] = bl.level
		}
		fmt.Printf("%-26s %3d %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f\n", bl.name, bl.n, bl.median, bl.lowest, bl.most,
			bl.peak, now, bl.level)
	}
	if !write {
		return
	}
	src, err := os.ReadFile(banksFile)
	if err != nil {
		log.Fatal(err)
	}
	if src, err = setLevels(src, "Level", levels); err != nil {
		log.Fatal(err)
	}
	if src, err = setLevels(src, "KitLevel", kits); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(banksFile, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
