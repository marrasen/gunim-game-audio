package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marrasen/gunim-game-audio/calls"
)

// render writes every call of lib to a WAV file in dir, mono, 24 bits,
// take seed, and says how each measures and where it falls short.
func render(lib *calls.Library, dir string, seed uint64) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var short int
	for _, c := range lib.Companions {
		for _, kind := range c.Kinds() {
			if _, ok := c.Calls[kind]; !ok {
				continue
			}
			t, err := lib.Take(c.ID, kind, seed)
			if err != nil {
				return err
			}
			name := filepath.Join(dir, calls.FileName(c.ID, kind, "wav"))
			if err := writeWAV(name, t.Samples); err != nil {
				return err
			}
			s := t.Stats
			fmt.Printf("%-24s %4.2f s  %5.1f LUFS  %5.1f dBTP  lows %2.0f%%  1–4 kHz %2.0f%%  limited %4.1f dB",
				filepath.Base(name), s.Length, s.Loudness, s.Peak, s.Lows*100, s.Presence*100, s.Limited)
			if p := s.Problems(kind, lib.Master); len(p) > 0 {
				short++
				fmt.Printf("  · %s", strings.Join(p, "; "))
			}
			fmt.Println()
		}
	}
	if short > 0 {
		return fmt.Errorf("%d calls fall short of what the game asks", short)
	}
	return nil
}

// writeWAV writes samples to a WAV file named name, as calls.WriteWAV
// does.
func writeWAV(name string, samples []float32) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	return errors.Join(calls.WriteWAV(f, samples), f.Close())
}
