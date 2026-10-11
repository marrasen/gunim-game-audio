package main

import (
	"slices"
	"sort"

	"github.com/marrasen/gunim-game-audio/midi"
)

// A Score is a song made ready to draw: its notes, each with its start
// and end, the instruments its channels play and when they change, and
// its beats.
type Score struct {
	Title  string
	Length float64
	// Notes are every note, in order of their start.
	Notes []Note
	// Programs are the instruments the channels change to, in order.
	Programs []ProgramAt
	// Beats are when each beat comes, in seconds, and BarBeats how many
	// beats a bar has.
	Beats    []float64
	BarBeats int
	BPM      float64
	// Channels say what each channel plays.
	Channels [16]ChannelInfo
	// Density is how busy each channel is through the song, in
	// densityBins slices of it, from 0 to 1.
	Density [][16]float32
	// MaxLen is how long the longest note lasts.
	MaxLen float64
	// index files the notes by the half seconds they sound in; span
	// makes it, on the window's side.
	index [][]int32
}

// Note is a note of a channel, from Start to End seconds.
type Note struct {
	Ch, Key, Vel uint8
	Start, End   float64
}

// ProgramAt is a channel's change of instrument, at Time seconds.
type ProgramAt struct {
	Time float64
	Ch   uint8
	Prog uint8
}

// ChannelInfo says what a channel plays.
type ChannelInfo struct {
	Notes int
	Drums bool
	// Low and High are its lowest and highest keys.
	Low, High uint8
}

// densityBins is how many slices of the song Density measures.
const densityBins = 480

// newScore makes f ready to draw.
func newScore(f *midi.File, title string) *Score {
	s := &Score{Title: title, Length: f.Length(), BarBeats: 4}
	evs := f.Events()
	type key struct{ ch, key uint8 }
	open := map[key][]int{}
	for i := range s.Channels {
		s.Channels[i].Low = 127
	}
	s.Channels[9].Drums = true
	end := func(k key, t float64) {
		if q := open[k]; len(q) > 0 {
			s.Notes[q[0]].End = t
			open[k] = q[1:]
		}
	}
	for _, e := range evs {
		ch := uint8(e.Channel())
		switch e.Kind() {
		case midi.NoteOn:
			if e.Data2 == 0 {
				end(key{ch, e.Data1}, e.Time)
				continue
			}
			k := key{ch, e.Data1}
			open[k] = append(open[k], len(s.Notes))
			s.Notes = append(s.Notes, Note{Ch: ch, Key: e.Data1, Vel: e.Data2, Start: e.Time, End: -1})
			c := &s.Channels[ch]
			c.Notes++
			c.Low, c.High = min(c.Low, e.Data1), max(c.High, e.Data1)
		case midi.NoteOff:
			end(key{ch, e.Data1}, e.Time)
		case midi.Program:
			s.Programs = append(s.Programs, ProgramAt{Time: e.Time, Ch: ch, Prog: e.Data1})
		case midi.Meta:
			if e.Data1 == 0x58 && len(e.Data) > 0 && e.Data[0] > 0 && s.BarBeats == 4 {
				s.BarBeats = int(e.Data[0])
			}
		}
	}
	for i := range s.Notes {
		n := &s.Notes[i]
		if n.End < 0 {
			n.End = s.Length
		}
		if s.Channels[n.Ch].Drums {
			// A drum's note off says nothing: it rings as it rings.
			n.End = n.Start + 0.15
		}
		n.End = max(n.End, n.Start+0.03)
		s.MaxLen = max(s.MaxLen, n.End-n.Start)
	}
	s.Length = max(s.Length, 0.1)
	s.beats(f, evs)
	s.density()
	return s
}

// beats finds when each beat comes, by the tempo map: f's ticks a beat,
// and the tempo events among evs.
func (s *Score) beats(f *midi.File, evs []midi.Event) {
	if f.Division <= 0 {
		s.BPM = 120
		for t := 0.0; t < s.Length; t += 0.5 {
			s.Beats = append(s.Beats, t)
		}
		return
	}
	// A beat each Division ticks: the time of each from the events'
	// ticks and times, between which the tempo holds.
	type stamp struct {
		tick int64
		t    float64
		us   int
	}
	stamps := []stamp{{0, 0, 500000}}
	for _, e := range evs {
		if us, ok := e.Tempo(); ok && us > 0 {
			stamps = append(stamps, stamp{e.Tick, e.Time, us})
		}
	}
	sort.SliceStable(stamps, func(i, j int) bool { return stamps[i].tick < stamps[j].tick })
	s.BPM = 60e6 / float64(stamps[min(1, len(stamps)-1)].us)
	div := int64(f.Division)
	si := 0
	for tick := int64(0); ; tick += div {
		for si+1 < len(stamps) && stamps[si+1].tick <= tick {
			si++
		}
		st := stamps[si]
		t := st.t + float64(tick-st.tick)*float64(st.us)/1e6/float64(div)
		if t > s.Length+0.01 || len(s.Beats) > 100000 {
			break
		}
		s.Beats = append(s.Beats, t)
	}
}

// density measures how busy each channel is in each slice of the song.
func (s *Score) density() {
	s.Density = make([][16]float32, densityBins)
	per := s.Length / densityBins
	for _, n := range s.Notes {
		a := int(n.Start / per)
		b := min(int(n.End/per), a+int(2/per)+1, densityBins-1)
		w := float32(n.Vel) / 127
		for i := max(a, 0); i <= b && i < densityBins; i++ {
			s.Density[i][n.Ch] += w
		}
	}
	var top float32
	for _, d := range s.Density {
		var sum float32
		for _, v := range d {
			sum += v
		}
		top = max(top, sum)
	}
	if top > 0 {
		for i := range s.Density {
			for c := range s.Density[i] {
				s.Density[i][c] /= top
			}
		}
	}
}

// programAt returns the instrument channel ch plays at t.
func (s *Score) programAt(ch int, t float64) int {
	p := 0
	for _, pa := range s.Programs {
		if pa.Time > t+0.001 {
			break
		}
		if int(pa.Ch) == ch {
			p = int(pa.Prog)
		}
	}
	return p
}

// firstProgram returns the first instrument channel ch plays.
func (s *Score) firstProgram(ch int) int {
	first := -1.0
	for _, n := range s.Notes {
		if int(n.Ch) == ch {
			first = n.Start
			break
		}
	}
	return s.programAt(ch, max(first, 0))
}

// bucketSecs is how long a slice of the song the note index files its
// notes by.
const bucketSecs = 0.5

// span returns the notes that sound between from and to: those that
// start before to and end after from. It looks them up in an index of
// the notes sounding in each half second, made on first use, so a long
// song costs no more than a short one.
func (s *Score) span(from, to float64, yield func(n *Note)) {
	if s.index == nil {
		s.makeIndex()
	}
	bf := max(int(from/bucketSecs), 0)
	bt := min(int(to/bucketSecs), len(s.index)-1)
	for b := bf; b <= bt; b++ {
		for _, i := range s.index[b] {
			n := &s.Notes[i]
			// Each note once: in the first bucket of the span it is in.
			if max(int(n.Start/bucketSecs), bf) != b {
				continue
			}
			if n.Start <= to && n.End >= from {
				yield(n)
			}
		}
	}
}

// makeIndex files each note under every half second it sounds in.
func (s *Score) makeIndex() {
	s.index = make([][]int32, int(s.Length/bucketSecs)+2)
	for i, n := range s.Notes {
		a := max(int(n.Start/bucketSecs), 0)
		b := min(int(n.End/bucketSecs), len(s.index)-1)
		for k := a; k <= b; k++ {
			s.index[k] = append(s.index[k], int32(i))
		}
	}
}

// beatAt returns the beat t falls in, from 0, and how far through it,
// from 0 to 1.
func (s *Score) beatAt(t float64) (int, float64) {
	if len(s.Beats) < 2 {
		return 0, 0
	}
	i := sort.SearchFloat64s(s.Beats, t)
	if i < len(s.Beats) && s.Beats[i] == t {
		return i, 0
	}
	i--
	if i < 0 {
		return 0, 0
	}
	if i >= len(s.Beats)-1 {
		d := s.Beats[len(s.Beats)-1] - s.Beats[len(s.Beats)-2]
		x := (t - s.Beats[i]) / d
		return i + int(x), x - float64(int(x))
	}
	return i, (t - s.Beats[i]) / (s.Beats[i+1] - s.Beats[i])
}

// used returns the channels with notes, in order.
func (s *Score) used() []int {
	var chs []int
	for i, c := range s.Channels {
		if c.Notes > 0 {
			chs = append(chs, i)
		}
	}
	return slices.Clip(chs)
}
