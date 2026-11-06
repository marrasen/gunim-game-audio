package soundfont

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/jfreymuth/oggvorbis"
)

// ErrTooLong says a sample holds more frames than Decode was asked to
// read.
var ErrTooLong = errors.New("soundfont: a sample too long to read")

// mostPairs is how many pairings of a preset's zones with its
// instruments' Zones looks through before it stops.
const mostPairs = 1 << 24

// A Zone is a sample a preset plays and the keys and velocities that
// play it, for a program that reads the whole of a preset, not a note.
type Zone struct {
	// KeyLo to KeyHi and VelLo to VelHi are the keys and velocities
	// that play it, inclusive.
	KeyLo, KeyHi, VelLo, VelHi int
	// Region is how it plays at its root, or its nearest key to that,
	// and at velocity 127, where a bank's velocity moves nothing.
	Region
	// HalfGain is its Gain at velocity 64.
	HalfGain float64
}

// Zones returns each zone of p, in the bank's order. A hostile bank can
// hold a great many, as each of a preset's zones pairs with each of its
// instrument's, so it builds them as they are asked for, the caller may
// stop, and it stops itself past far more pairings than a real bank has.
func (f *Font) Zones(p *Preset) iter.Seq[Zone] {
	return func(yield func(Zone) bool) {
		pairs := 0
		for i := range p.zones {
			pz := &p.zones[i]
			if pz.link < 0 || pz.link >= len(f.instruments) {
				continue
			}
			in := &f.instruments[pz.link]
			for j := range in.zones {
				if pairs++; pairs > mostPairs {
					return
				}
				iz := &in.zones[j]
				z := Zone{KeyLo: max(pz.keyLo, iz.keyLo), KeyHi: min(pz.keyHi, iz.keyHi),
					VelLo: max(pz.velLo, iz.velLo), VelHi: min(pz.velHi, iz.velHi)}
				if iz.link < 0 || iz.link >= len(f.samples) || z.KeyLo > z.KeyHi || z.VelLo > z.VelHi {
					continue
				}
				key := z.KeyLo
				if z.KeyLo != z.KeyHi {
					key = min(max(f.region(z.KeyLo, 127, &in.global, iz, &p.global, pz).Root, z.KeyLo), z.KeyHi)
				}
				z.Region = f.region(key, 127, &in.global, iz, &p.global, pz)
				z.HalfGain = f.region(key, 64, &in.global, iz, &p.global, pz).Gain
				if !yield(z) {
					return
				}
			}
		}
	}
}

// Decode returns the sample's frames as Frames does, but says why it
// has none, and keeps nothing: a program that reads every sample of a
// bank once holds only the ones it is reading. It stops with ErrTooLong
// past limit frames, as a small Ogg Vorbis stream can unpack to any
// length.
func (s *Sample) Decode(limit int) ([]int16, error) {
	d := s.f.data
	if !s.vorbis {
		if s.Start < 0 || s.End > len(d)/2 || s.Start >= s.End {
			return nil, fmt.Errorf("soundfont: sample %q lies outside the bank", s.Name)
		}
		if s.End-s.Start > limit {
			return nil, ErrTooLong
		}
		return pcm(d, s.Start, s.End), nil
	}
	if s.Start < 0 || s.End > len(d) || s.Start >= s.End {
		return nil, fmt.Errorf("soundfont: sample %q lies outside the bank", s.Name)
	}
	r, err := oggvorbis.NewReader(bytes.NewReader(d[s.Start:s.End]))
	if err != nil {
		return nil, fmt.Errorf("soundfont: sample %q: %w", s.Name, err)
	}
	ch := r.Channels()
	if ch < 1 {
		return nil, fmt.Errorf("soundfont: sample %q has no channels", s.Name)
	}
	buf := make([]float32, 4096*ch)
	var out []int16
	for {
		n, err := r.Read(buf)
		if len(out)+n/ch > limit {
			return nil, ErrTooLong
		}
		for i := 0; i+ch <= n; i += ch {
			out = append(out, int16(min(max(buf[i]*32767, -32768), 32767)))
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("soundfont: sample %q: %w", s.Name, err)
		}
	}
}

// Close releases the file Open mapped, so it can be moved or removed.
// Nothing read from the bank may be used after it: not its samples, nor
// frames Frames or Decode returned. A bank held in memory has nothing
// to release.
func (f *Font) Close() error {
	f.data = nil
	release := f.release
	f.release = nil
	if release == nil {
		return nil
	}
	return release()
}
