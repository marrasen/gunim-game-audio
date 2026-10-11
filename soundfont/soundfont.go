// Package soundfont reads SoundFont banks of sampled instruments, the
// SF2 files of E-mu's SoundFont 2 and the SF3 files of MuSE and
// MuseScore, whose samples are Ogg Vorbis, and finds the samples a note
// of an instrument plays, with how to play them.
//
//	f, err := soundfont.Open("MuseScore_General.sf3")
//	for _, r := range f.Regions(0, 0, 60, 100) { // a piano's middle C
//		...
//	}
//
// No bank comes with the package: each has a licence of its own. See
// Banks for some that may be fetched, and what they allow.
package soundfont

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/jfreymuth/oggvorbis"
)

// A Font is a SoundFont bank, read.
type Font struct {
	// Name is the bank's name, Copyright and Comment what it says of
	// itself.
	Name, Copyright, Comment string
	Presets                  []Preset
	instruments              []instrument
	samples                  []*Sample
	// data is the bank's sample data: 16-bit samples for an SF2, the Ogg
	// Vorbis streams for an SF3.
	data []byte
	// byKey finds a preset by bank and program.
	byKey map[int]*Preset
}

// A Preset is an instrument of the bank as a MIDI program plays it: by
// its Bank, 128 for the drum kits, and Program.
type Preset struct {
	Name          string
	Bank, Program int
	zones         []zone
	global        zone
}

type instrument struct {
	name   string
	zones  []zone
	global zone
}

// zone is a zone of a preset or an instrument: its generators, by number,
// as the file gives them.
type zone struct {
	gens map[int]int16
	// keys and vels are its ranges, inclusive.
	keyLo, keyHi, velLo, velHi int
	// link is the instrument a preset's zone plays, or the sample an
	// instrument's zone plays, -1 for a global zone.
	link int
}

// A Sample is a sample of the bank.
type Sample struct {
	Name string
	// Rate is its sample rate, Root the key it plays at its rate, and
	// Correction how many cents it is out.
	Rate, Root, Correction int
	// Start, End, LoopStart and LoopEnd are where it lies, in frames of
	// the bank's data, or, for an SF3's, in the bytes of its stream, its
	// loop in frames from its start.
	Start, End, LoopStart, LoopEnd int
	// vorbis says it is an Ogg Vorbis stream, decoded on first use.
	vorbis bool
	once   sync.Once
	frames []int16
	f      *Font
}

// Frames returns the sample's frames, 16 bits each, decoding them first
// where the bank holds them compressed.
func (s *Sample) Frames() []int16 {
	s.once.Do(func() {
		d := s.f.data
		if !s.vorbis {
			lo, hi := max(s.Start, 0), min(s.End, len(d)/2)
			if lo >= hi {
				return
			}
			s.frames = make([]int16, hi-lo)
			for i := range s.frames {
				s.frames[i] = int16(binary.LittleEndian.Uint16(d[2*(lo+i):]))
			}
			return
		}
		lo, hi := max(s.Start, 0), min(s.End, len(d))
		if lo >= hi {
			return
		}
		pcm, format, err := oggvorbis.ReadAll(bytes.NewReader(d[lo:hi]))
		if err != nil || format == nil || format.Channels == 0 {
			return
		}
		ch := format.Channels
		s.frames = make([]int16, len(pcm)/ch)
		for i := range s.frames {
			v := pcm[i*ch] * 32767
			s.frames[i] = int16(min(max(v, -32768), 32767))
		}
	})
	return s.frames
}

// Open reads the bank in the file at path.
func Open(path string) (*Font, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(bytes.NewReader(b))
}

// Read reads an SF2 or an SF3 bank.
func Read(r io.Reader) (*Font, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "sfbk" {
		return nil, errors.New("soundfont: not a SoundFont bank")
	}
	f := &Font{byKey: map[int]*Preset{}}
	var pdta map[string][]byte
	err = chunks(b[12:], func(id string, body []byte) error {
		if id != "LIST" || len(body) < 4 {
			return nil
		}
		kind, items := string(body[:4]), body[4:]
		switch kind {
		case "INFO":
			return chunks(items, func(id string, v []byte) error {
				s := strings.TrimRight(string(v), "\x00 ")
				switch id {
				case "INAM":
					f.Name = s
				case "ICOP":
					f.Copyright = s
				case "ICMT":
					f.Comment = s
				}
				return nil
			})
		case "sdta":
			return chunks(items, func(id string, v []byte) error {
				if id == "smpl" {
					f.data = v
				}
				return nil
			})
		case "pdta":
			pdta = map[string][]byte{}
			return chunks(items, func(id string, v []byte) error {
				pdta[id] = v
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if pdta == nil {
		return nil, errors.New("soundfont: no presets")
	}
	if err := f.readHydra(pdta); err != nil {
		return nil, err
	}
	return f, nil
}

// chunks calls fn with each RIFF chunk of b.
func chunks(b []byte, fn func(id string, body []byte) error) error {
	for len(b) >= 8 {
		id := string(b[:4])
		n := int(binary.LittleEndian.Uint32(b[4:8]))
		if n < 0 || 8+n > len(b) {
			n = len(b) - 8
		}
		if err := fn(id, b[8:8+n]); err != nil {
			return err
		}
		n += n & 1
		if 8+n > len(b) {
			break
		}
		b = b[8+n:]
	}
	return nil
}

// readHydra reads the presets, instruments and samples, the "hydra" of
// nine lists that point into each other.
func (f *Font) readHydra(p map[string][]byte) error {
	rec := func(id string, size int) [][]byte {
		d := p[id]
		var out [][]byte
		for i := 0; i+size <= len(d); i += size {
			out = append(out, d[i:i+size])
		}
		return out
	}
	u16 := func(b []byte, at int) int { return int(binary.LittleEndian.Uint16(b[at:])) }
	u32 := func(b []byte, at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }
	name := func(b []byte) string {
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return strings.TrimSpace(string(b))
	}
	phdr, pbag, pgen := rec("phdr", 38), rec("pbag", 4), rec("pgen", 4)
	inst, ibag, igen := rec("inst", 22), rec("ibag", 4), rec("igen", 4)
	shdr := rec("shdr", 46)
	if len(phdr) < 2 || len(inst) < 2 || len(shdr) < 1 {
		return errors.New("soundfont: an empty bank")
	}
	// zones reads the zones from bag a to b, their generators from the
	// generator list, the last generator, link, naming what each plays.
	zones := func(bags [][]byte, gens [][]byte, a, b, link int) (global zone, out []zone) {
		global = zone{link: -1, keyHi: 127, velHi: 127}
		for k := a; k < b && k+1 < len(bags); k++ {
			z := zone{gens: map[int]int16{}, keyHi: 127, velHi: 127, link: -1}
			g0, g1 := u16(bags[k], 0), u16(bags[k+1], 0)
			for g := g0; g < g1 && g < len(gens); g++ {
				op := u16(gens[g], 0)
				amt := gens[g][2:4]
				switch op {
				case 43:
					z.keyLo, z.keyHi = int(amt[0]), int(amt[1])
				case 44:
					z.velLo, z.velHi = int(amt[0]), int(amt[1])
				case link:
					z.link = u16(gens[g], 2)
				default:
					z.gens[op] = int16(binary.LittleEndian.Uint16(amt))
				}
			}
			if z.link < 0 {
				// A zone that plays nothing is the global zone, if first.
				if k == a {
					global = z
				}
				continue
			}
			out = append(out, z)
		}
		return global, out
	}
	for i := 0; i+1 < len(inst); i++ {
		g, zs := zones(ibag, igen, u16(inst[i], 20), u16(inst[i+1], 20), 53)
		f.instruments = append(f.instruments, instrument{name: name(inst[i][:20]), zones: zs, global: g})
	}
	for i := 0; i+1 < len(phdr); i++ {
		h := phdr[i]
		g, zs := zones(pbag, pgen, u16(h, 24), u16(phdr[i+1], 24), 41)
		f.Presets = append(f.Presets, Preset{Name: name(h[:20]), Program: u16(h, 20), Bank: u16(h, 22), zones: zs, global: g})
	}
	for i := range f.Presets {
		pr := &f.Presets[i]
		f.byKey[pr.Bank<<8|pr.Program] = pr
	}
	for _, h := range shdr {
		s := &Sample{Name: name(h[:20]), Start: u32(h, 20), End: u32(h, 24), LoopStart: u32(h, 28), LoopEnd: u32(h, 32),
			Rate: u32(h, 36), Root: int(h[40]), Correction: int(int8(h[41])), f: f}
		kind := u16(h, 44)
		s.vorbis = kind&0x10 != 0
		if !s.vorbis {
			// An SF2's loop is in frames of the data, as its start is.
			s.LoopStart -= s.Start
			s.LoopEnd -= s.Start
		}
		if s.Rate <= 0 {
			s.Rate = 44100
		}
		if s.Root > 127 {
			s.Root = 60
		}
		f.samples = append(f.samples, s)
	}
	sort.SliceStable(f.Presets, func(i, j int) bool {
		a, b := f.Presets[i], f.Presets[j]
		if a.Bank != b.Bank {
			return a.Bank < b.Bank
		}
		return a.Program < b.Program
	})
	for i := range f.Presets {
		pr := &f.Presets[i]
		f.byKey[pr.Bank<<8|pr.Program] = pr
	}
	return nil
}

// Preset returns the preset of bank and program, or for a bank the font
// lacks, the program of bank 0, or of the drum bank 128 for a drum
// bank; for a drum kit it lacks, the standard kit, 0.
func (f *Font) Preset(bank, program int) *Preset {
	if p := f.byKey[bank<<8|program]; p != nil {
		return p
	}
	if bank == 128 {
		if p := f.byKey[128<<8]; p != nil {
			return p
		}
		return nil
	}
	return f.byKey[program]
}

// A Region is a sample a note plays, and how: all its generators
// summed, the preset's onto the instrument's, in the units a player
// works in.
type Region struct {
	Sample *Sample
	// Root is the key the sample plays at its rate, Tune how many cents
	// it is moved, and KeyTrack how many cents each key moves it, 100 as
	// a piano's do.
	Root     int
	Tune     float64
	KeyTrack float64
	// Start and End bound what of the sample plays, LoopStart and
	// LoopEnd its loop, all in its frames; Loop is 0 for none, 1 to loop
	// on, and 3 to loop until let go and then play on to the end.
	Start, End, LoopStart, LoopEnd int
	Loop                           int
	// Gain is its level, from its attenuation, and Pan its place, -1 to
	// 1.
	Gain, Pan float64
	// Vol is its volume envelope, the sustain a level from 0 to 1, and
	// Mod its modulation envelope, which moves its pitch by ModToPitch
	// cents and its filter by ModToFilter cents at its peak.
	Vol, Mod                Env
	ModToPitch, ModToFilter float64
	// Cutoff is its lowpass filter's cutoff, in hertz, and Q its
	// resonance, in decibels.
	Cutoff, Q float64
	// VibHz, VibDelay and VibToPitch are its vibrato LFO, in hertz,
	// seconds and cents; ModHz and ModDelay its modulation LFO, which
	// moves its pitch, filter and volume by ModLFOToPitch cents,
	// ModLFOToFilter cents and ModLFOToVolume decibels.
	VibHz, VibDelay, VibToPitch                   float64
	ModHz, ModDelay                               float64
	ModLFOToPitch, ModLFOToFilter, ModLFOToVolume float64
	// Reverb and Chorus are what it sends to them, 0 to 1.
	Reverb, Chorus float64
	// Exclusive is its class: a note of a class cuts off the notes of the
	// same class playing, as a closed hi-hat an open one; 0 for none.
	Exclusive int
}

// Env is an envelope: Delay, Attack, Hold, Decay and Release in seconds,
// and Sustain the level it holds.
type Env struct{ Delay, Attack, Hold, Decay, Sustain, Release float64 }

// Regions returns the regions key plays at vel, 0 to 127, of the preset
// of bank and program, as Preset finds it: nil where it finds none.
func (f *Font) Regions(bank, program, key, vel int) []Region {
	p := f.Preset(bank, program)
	if p == nil {
		return nil
	}
	var out []Region
	for _, pz := range p.zones {
		if key < pz.keyLo || key > pz.keyHi || vel < pz.velLo || vel > pz.velHi || pz.link >= len(f.instruments) {
			continue
		}
		in := &f.instruments[pz.link]
		for _, iz := range in.zones {
			if key < iz.keyLo || key > iz.keyHi || vel < iz.velLo || vel > iz.velHi || iz.link >= len(f.samples) {
				continue
			}
			out = append(out, f.region(key, vel, &in.global, &iz, &p.global, &pz))
		}
	}
	return out
}

// Generator numbers, as the SoundFont 2.04 specification gives them.
const (
	gStartOff, gEndOff, gLoopStartOff, gLoopEndOff = 0, 1, 2, 3
	gStartCoarse                                   = 4
	gModLFOToPitch, gVibLFOToPitch, gModEnvToPitch = 5, 6, 7
	gFilterFc, gFilterQ                            = 8, 9
	gModLFOToFilter, gModEnvToFilter               = 10, 11
	gEndCoarse                                     = 12
	gModLFOToVolume                                = 13
	gChorus, gReverb, gPan                         = 15, 16, 17
	gModLFODelay, gModLFOFreq                      = 21, 22
	gVibLFODelay, gVibLFOFreq                      = 23, 24
	gModEnvDelay                                   = 25
	gKeyToModEnvHold, gKeyToModEnvDecay            = 31, 32
	gVolEnvDelay                                   = 33
	gKeyToVolEnvHold, gKeyToVolEnvDecay            = 39, 40
	gLoopStartCoarse                               = 45
	gKeynum, gVelocity, gAttenuation               = 46, 47, 48
	gLoopEndCoarse                                 = 50
	gCoarseTune, gFineTune                         = 51, 52
	gSampleModes                                   = 54
	gScaleTuning, gExclusive, gRootKey             = 56, 57, 58
)

// defaults are the generators' values where no zone sets them.
var defaults = map[int]int16{
	gFilterFc: 13500, gModLFODelay: -12000, gVibLFODelay: -12000,
	gModEnvDelay: -12000, gModEnvDelay + 1: -12000, gModEnvDelay + 2: -12000, gModEnvDelay + 3: -12000, gModEnvDelay + 5: -12000,
	gVolEnvDelay: -12000, gVolEnvDelay + 1: -12000, gVolEnvDelay + 2: -12000, gVolEnvDelay + 3: -12000, gVolEnvDelay + 5: -12000,
	gKeynum: -1, gVelocity: -1, gScaleTuning: 100, gRootKey: -1,
}

// region sums an instrument's zone over its global zone, and a preset's
// over its own, onto it, and works out what the sums mean.
func (f *Font) region(key, vel int, ig, iz, pg, pz *zone) Region {
	inst := func(op int) float64 {
		if v, ok := iz.gens[op]; ok {
			return float64(v)
		}
		if v, ok := ig.gens[op]; ok {
			return float64(v)
		}
		return float64(defaults[op])
	}
	pre := func(op int) float64 {
		if v, ok := pz.gens[op]; ok {
			return float64(v)
		}
		if v, ok := pg.gens[op]; ok {
			return float64(v)
		}
		return 0
	}
	// A preset's generators add to its instrument's, where they may.
	g := func(op int) float64 { return inst(op) + pre(op) }
	s := f.samples[iz.link]
	r := Region{Sample: s}
	if k := int(inst(gKeynum)); k >= 0 {
		key = k
	}
	r.Root = s.Root
	if rk := int(inst(gRootKey)); rk >= 0 {
		r.Root = rk
	}
	r.KeyTrack = g(gScaleTuning)
	r.Tune = g(gCoarseTune)*100 + g(gFineTune) + float64(s.Correction)
	// Offsets into the sample, fine and coarse, from where the sample
	// header puts it.
	off := func(fine, coarse int) int { return int(inst(fine)) + int(inst(coarse))*32768 }
	r.Start = off(gStartOff, gStartCoarse)
	r.LoopStart = s.LoopStart + off(gLoopStartOff, gLoopStartCoarse)
	r.LoopEnd = s.LoopEnd + off(gLoopEndOff, gLoopEndCoarse)
	r.End = off(gEndOff, gEndCoarse) // from the sample's end; worked out on play
	r.Loop = int(inst(gSampleModes)) & 3
	if r.Loop == 2 {
		r.Loop = 0
	}
	// E-mu's hardware took the attenuation at 0.4 of its face value, and
	// the banks are voiced so.
	r.Gain = math.Pow(10, -g(gAttenuation)*0.4/200)
	r.Pan = min(max(g(gPan)/500, -1), 1)
	tc := func(v float64) float64 { return math.Pow(2, v/1200) }
	// The envelopes: hold and decay shorten as keys climb, where the bank
	// says.
	envOf := func(base, keyHold, keyDecay int, sustainDB bool) Env {
		e := Env{Delay: tc(g(base)), Attack: tc(g(base + 1)),
			Hold:    tc(g(base+2) + g(keyHold)*float64(60-key)),
			Decay:   tc(g(base+3) + g(keyDecay)*float64(60-key)),
			Release: tc(g(base + 5))}
		sus := g(base + 4)
		if sustainDB {
			e.Sustain = math.Pow(10, -min(max(sus, 0), 1440)/200)
		} else {
			e.Sustain = 1 - min(max(sus, 0), 1000)/1000
		}
		return e
	}
	r.Vol = envOf(gVolEnvDelay, gKeyToVolEnvHold, gKeyToVolEnvDecay, true)
	r.Mod = envOf(gModEnvDelay, gKeyToModEnvHold, gKeyToModEnvDecay, false)
	r.ModToPitch, r.ModToFilter = g(gModEnvToPitch), g(gModEnvToFilter)
	r.Cutoff = 8.176 * math.Pow(2, min(max(g(gFilterFc), 1500), 13500)/1200)
	r.Q = max(g(gFilterQ)/10, 0)
	r.VibHz, r.VibDelay, r.VibToPitch = 8.176*tc(g(gVibLFOFreq)), tc(g(gVibLFODelay)), g(gVibLFOToPitch)
	r.ModHz, r.ModDelay = 8.176*tc(g(gModLFOFreq)), tc(g(gModLFODelay))
	r.ModLFOToPitch, r.ModLFOToFilter, r.ModLFOToVolume = g(gModLFOToPitch), g(gModLFOToFilter), g(gModLFOToVolume)/10
	r.Reverb, r.Chorus = min(max(g(gReverb)/1000, 0), 1), min(max(g(gChorus)/1000, 0), 1)
	r.Exclusive = int(inst(gExclusive))
	return r
}

// Preload decodes the samples of the preset of bank and program, as
// Preset finds it, so a player's first notes of it need not wait.
func (f *Font) Preload(bank, program int) {
	p := f.Preset(bank, program)
	if p == nil {
		return
	}
	for _, pz := range p.zones {
		if pz.link >= len(f.instruments) {
			continue
		}
		for _, iz := range f.instruments[pz.link].zones {
			if iz.link < len(f.samples) {
				f.samples[iz.link].Frames()
			}
		}
	}
}

// String names the font, for a list.
func (f *Font) String() string {
	return fmt.Sprintf("%s (%d presets, %d samples)", f.Name, len(f.Presets), len(f.samples))
}
