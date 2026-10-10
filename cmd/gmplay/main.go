// Command gmplay plays a MIDI file through the synth engine's General
// MIDI instruments, until it ends or is interrupted.
//
//	go run github.com/marrasen/gunim-game-audio/cmd/gmplay@latest song.mid
//
// -o renders the file to a WAV file in place of playing it, and says
// how loud it is. -loop plays it again from its start each time it
// ends, and -start starts it that many seconds in. -v lists the
// instruments each channel plays.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"

	"github.com/marrasen/gunim-game-audio/midi"
	"github.com/marrasen/gunim-game-audio/synth"
)

func main() {
	out := flag.String("o", "", "render to this WAV file, in place of playing")
	loop := flag.Bool("loop", false, "play the file again each time it ends")
	start := flag.Float64("start", 0, "start this many seconds in")
	gain := flag.Float64("gain", 0, "turn the mix up or down by this many decibels")
	verbose := flag.Bool("v", false, "list the instruments each channel plays")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: gmplay [flags] file.mid\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	f, err := load(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	name := f.Name()
	if name == "" {
		name = filepath.Base(flag.Arg(0))
	}
	fmt.Printf("%s: format %d, %d tracks, %s\n", name, f.Format, len(f.Tracks), clock(f.Length()))
	if *verbose {
		instruments(f)
	}
	p := synth.NewMIDIPlayer(f)
	p.GM().Gain *= float32(math.Pow(10, *gain/20))
	if *start > 0 {
		p.Seek(*start)
	}
	if *out != "" {
		if err := render(p, *out); err != nil {
			log.Fatal(err)
		}
		return
	}
	p.SetLoop(*loop)
	mix := audio.NewMixer()
	if _, err := speaker.Open(mix, speaker.Options{Name: "gmplay"}); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	v := mix.Play(p, audio.Options{})
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return
		case <-v.Done():
			fmt.Println()
			if err := v.Err(); err != nil && !errors.Is(err, io.EOF) {
				log.Fatal(err)
			}
			return
		case <-tick.C:
			fmt.Printf("\r%s / %s ", clock(p.Position()), clock(p.Length()))
		}
	}
}

// load reads the MIDI file at path.
func load(path string) (*midi.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return midi.Read(file)
}

// render renders p to a WAV file, and prints its loudness.
func render(p *synth.MIDIPlayer, path string) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	w, err := audio.NewWAVWriter(file, audio.SampleRate, 24, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, w.Close()) }()
	var m audio.LoudnessMeter
	buf := make([]float32, 2*1024)
	began := time.Now()
	for {
		n, rerr := p.Read(buf)
		m.Write(buf[:2*n])
		if err := w.Write(buf[:2*n]); err != nil {
			return err
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	lufs, _ := m.Integrated()
	fmt.Printf("%s: %s in %.1f s, %.1f LUFS, peak %.1f dB\n", path, clock(p.Position()),
		time.Since(began).Seconds(), lufs, 20*math.Log10(float64(m.Peak())+1e-9))
	return nil
}

// instruments prints the instruments each channel plays, in order of
// first use.
func instruments(f *midi.File) {
	used := map[int][]string{}
	notes := map[int]bool{}
	seen := map[[2]int]bool{}
	for _, e := range f.Events() {
		ch := e.Channel()
		switch e.Kind() {
		case midi.Program:
			if ch == 9 {
				continue
			}
			k := [2]int{ch, int(e.Data1)}
			if !seen[k] {
				seen[k] = true
				used[ch] = append(used[ch], synth.GMNames[e.Data1])
			}
		case midi.NoteOn:
			if e.Data2 > 0 {
				notes[ch] = true
			}
		}
	}
	var chs []int
	for ch := range notes {
		chs = append(chs, ch)
	}
	sort.Ints(chs)
	for _, ch := range chs {
		switch {
		case ch == 9:
			fmt.Printf("  channel 10: drums\n")
		case len(used[ch]) == 0:
			fmt.Printf("  channel %d: %s\n", ch+1, synth.GMNames[0])
		default:
			fmt.Printf("  channel %d: %v\n", ch+1, used[ch])
		}
	}
}

// clock writes secs as minutes and seconds.
func clock(secs float64) string {
	s := int(secs)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
