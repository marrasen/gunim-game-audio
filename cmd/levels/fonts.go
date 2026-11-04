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
	"github.com/marrasen/gunim-game-audio/synth"
)

// bankLevel is a bank measured: its sounds' loudness at 0 dB, their median, and the loudest true peak.
type bankLevel struct {
	bank         soundfont.Bank
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

// measureBank plays every sound of bank b, but its kits, at 0 dB and returns how loud they stand.
func measureBank(b soundfont.Bank, verbose bool) (bankLevel, bool) {
	path, ok := fontcache.Fetched(b)
	if !ok {
		log.Printf("%s is not fetched: skipped", b.Name)
		return bankLevel{}, false
	}
	f, err := soundfont.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	// A sound reads its samples on a goroutine as it starts; read them all first, so the render hears them.
	for _, p := range f.Presets {
		if p.Bank != synth.DrumBank {
			f.Preload(p.Bank, p.Program)
		}
	}
	name := b.Key()
	if b.Group == "" {
		name = music.SoundFont
	}
	plugsynth.UseFont(plugsynth.Font{Name: name, Title: b.Name, Group: b.Group, Kit: b.Kit, Font: f})
	defer plugsynth.DropFont(name)
	var sounds []plugsynth.Sound
	for _, s := range plugsynth.Sounds() {
		if s.Group == plugsynth.GMDrums {
			continue
		}
		// One bank is used at a time, so the recorded instruments are b's.
		if b.Group == "" && s.Bank == b.Name || b.Group != "" && s.Bank == plugsynth.Recorded {
			sounds = append(sounds, s)
		}
	}
	if len(sounds) == 0 {
		log.Fatalf("%s has no sounds to measure", b.Name)
	}
	res := measureSounds(sounds)
	bl := bankLevel{bank: b, peak: math.Inf(-1), lowest: math.Inf(1), most: math.Inf(-1)}
	var ls []float64
	for i, r := range res {
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
		bl.peak = max(bl.peak, r.peak)
		bl.lowest, bl.most = min(bl.lowest, r.lufs), max(bl.most, r.lufs)
	}
	if len(ls) == 0 {
		log.Fatalf("%s plays nothing", b.Name)
	}
	slices.Sort(ls)
	bl.n = len(ls)
	bl.median = ls[len(ls)/2]
	return bl, true
}

// wantLevel is the Level bank b wants, to half a decibel. A General MIDI bank holds every kind of sound, some far
// from the others, so only its median goes to the target. A recorded instrument is one sound, and its peak may come
// first.
func wantLevel(b soundfont.Bank, median, peak float64) float64 {
	want := math.Round(2*(plugsynth.LevelTarget-median)) / 2
	if b.Group != "" {
		// Rounded down, so the peak stays under LevelPeak.
		want = min(want, math.Floor(2*(plugsynth.LevelPeak-peak))/2)
	}
	return want
}

// setLevels returns the source of banks.go with each bank named in levels set to its Level. It formats what it
// returns.
func setLevels(src []byte, levels map[string]float64) ([]byte, error) {
	for name, level := range levels {
		re := regexp.MustCompile(`(?m)(^\s*Name:\s*"` + regexp.QuoteMeta(name) + `",.*\n)(\s*Level:.*\n)?`)
		if len(re.FindAll(src, -1)) != 1 {
			return nil, fmt.Errorf("no one bank named %q", name)
		}
		line := fmt.Sprintf("Level: %s,\n", strconv.FormatFloat(level, 'f', -1, 64))
		src = re.ReplaceAllFunc(src, func(m []byte) []byte {
			return append(re.ReplaceAll(m, []byte("$1")), line...)
		})
	}
	return format.Source(src)
}

// banksFile is where the banks' Levels are, from the module's root.
const banksFile = "soundfont/banks.go"

// fonts measures every bank of General MIDI and of the recorded instruments, but the kits, and prints the Level each
// wants. With write it sets those Levels in banksFile.
func fonts(verbose, write bool) {
	fmt.Printf("Target %.1f LUFS, peaks to %.1f dBFS. Each sound plays its level phrase at 0 dB.\n\n",
		plugsynth.LevelTarget, plugsynth.LevelPeak)
	var out []bankLevel
	for _, b := range slices.Concat(soundfont.Banks, soundfont.Instruments) {
		if b.Kit {
			continue
		}
		if verbose {
			fmt.Println(b.Name)
		}
		bl, ok := measureBank(b, verbose)
		if !ok {
			continue
		}
		bl.level = wantLevel(b, bl.median, bl.peak)
		out = append(out, bl)
	}
	fmt.Printf("\n%-26s %3s %8s %8s %8s %8s %8s %8s\n", "bank", "n", "median", "low", "high", "peak", "Level", "set")
	levels := map[string]float64{}
	for _, bl := range out {
		fmt.Printf("%-26s %3d %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f\n", bl.bank.Name, bl.n, bl.median, bl.lowest, bl.most,
			bl.peak, bl.bank.Level, bl.level)
		levels[bl.bank.Name] = bl.level
	}
	if !write {
		return
	}
	src, err := os.ReadFile(banksFile)
	if err != nil {
		log.Fatal(err)
	}
	src, err = setLevels(src, levels)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(banksFile, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
