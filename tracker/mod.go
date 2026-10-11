package tracker

import (
	"errors"
	"math"
	"strconv"
)

// modClock is the PAL Amiga's clock, doubled and in FastTracker's
// periods, four of the Amiga's: a sample's rate is it over the period.
const modClock = 14187578.4

// modChannels returns how many channels a MOD's signature says it has,
// or 0 for a signature it is not.
func modChannels(sig string) int {
	switch sig {
	case "M.K.", "M!K!", "M&K!", "FLT4", "4CHN", "N.T.":
		return 4
	case "6CHN":
		return 6
	case "8CHN", "CD81", "OKTA", "OCTA", "FLT8":
		return 8
	}
	if len(sig) == 4 && (sig[2:] == "CH" || sig[2:] == "CN") {
		if n, err := strconv.Atoi(sig[:2]); err == nil && n > 0 && n <= 32 {
			return n
		}
	}
	if len(sig) == 4 && sig[:3] == "TDZ" && sig[3] >= '1' && sig[3] <= '9' {
		return int(sig[3] - '0')
	}
	if len(sig) == 4 && sig[1:] == "CHN" && sig[0] >= '1' && sig[0] <= '9' {
		return int(sig[0] - '0')
	}
	return 0
}

// readMOD reads a MOD of 31 samples, which says how many channels it has
// at byte 1080.
func readMOD(b []byte) (*Module, error) {
	channels := modChannels(string(b[1080:1084]))
	if channels == 0 {
		return nil, errNotMOD
	}
	return readMODBody(b, 31, channels, 950, 1084)
}

// readMOD15 reads a MOD of 15 samples, as the Soundtracker made, of 4
// channels and no signature, after checking it looks like one.
func readMOD15(b []byte) (*Module, error) {
	if len(b) < 600+1024 {
		return nil, errors.New("tracker: too short for a MOD")
	}
	for i := range 15 {
		h := b[20+30*i : 50+30*i]
		if h[25] > 64 {
			return nil, errors.New("tracker: not a MOD")
		}
		for _, c := range h[:22] {
			if c != 0 && (c < 32 || c > 126) {
				return nil, errors.New("tracker: not a MOD")
			}
		}
	}
	if b[470] == 0 || b[470] > 128 {
		return nil, errors.New("tracker: not a MOD")
	}
	m, err := readMODBody(b, 15, 4, 470, 600)
	if err == nil {
		m.Tracker = "Soundtracker"
	}
	return m, err
}

// readMODBody reads a MOD of samples samples and channels channels,
// its song's length at songAt and its patterns from patAt.
func readMODBody(b []byte, samples, channels, songAt, patAt int) (*Module, error) {
	r := &reader{b: b, ok: true}
	m := &Module{Format: "MOD", Tracker: "ProTracker", Channels: channels, Speed: 6, BPM: 125, clock: modClock, OctaveShift: -2}
	m.Title = r.str(20)
	type head struct{ length, loopStart, loopLen int }
	heads := make([]head, samples)
	for i := range samples {
		s := Sample{Name: r.str(22), Bits: 8}
		h := head{length: r.u16be() * 2}
		ft := r.u8() & 0x0F
		if ft >= 8 {
			ft -= 16
		}
		s.Finetune = ft * 16
		s.Volume = min(r.u8(), 64)
		h.loopStart, h.loopLen = r.u16be()*2, r.u16be()*2
		heads[i] = h
		in := Instrument{Name: s.Name, Samples: []Sample{s}}
		m.Instruments = append(m.Instruments, in)
	}
	r.at = songAt
	songLen := r.u8()
	restart := r.u8()
	orders := r.take(128)
	if songLen == 0 || songLen > 128 {
		return nil, errors.New("tracker: a MOD of no song")
	}
	patterns := 0
	for _, o := range orders {
		patterns = max(patterns, int(o)+1)
	}
	for _, o := range orders[:songLen] {
		m.Orders = append(m.Orders, int(o))
	}
	if restart < songLen && restart != 127 {
		m.Restart = restart
	}
	r.at = patAt
	for range patterns {
		p := Pattern{Rows: 64, Cells: make([]Cell, 64*channels)}
		for i := range p.Cells {
			c := r.take(4)
			period := int(c[0]&0x0F)<<8 | int(c[1])
			cell := Cell{Instrument: c[0]&0xF0 | c[2]>>4, Effect: c[2] & 0x0F, Param: c[3]}
			if period > 0 {
				cell.Note = periodNote(period)
			}
			p.Cells[i] = cell
		}
		m.Patterns = append(m.Patterns, p)
	}
	if !r.ok {
		return nil, errors.New("tracker: a MOD cut short in its patterns")
	}
	// The samples follow, each 8 bits, signed.
	for i, h := range heads {
		data := r.take(min(h.length, len(r.b)-r.at))
		s := &m.Instruments[i].Samples[0]
		s.Data = make([]float32, len(data))
		for k, v := range data {
			s.Data[k] = float32(int8(v)) / 128
		}
		ls, ll := h.loopStart, h.loopLen
		// Some old files give the loop's start in bytes, not words.
		if ls+ll > len(data) && ls/2+ll <= len(data) {
			ls /= 2
		}
		if ll > 2 && ls < len(data) {
			s.LoopStart, s.LoopLen, s.Loop = ls, min(ll, len(data)-ls), 1
		}
	}
	m.Pan = make([]int, channels)
	for ch := range channels {
		// The Amiga plays channels 1 and 4 left and 2 and 3 right; here
		// they lean rather than sit hard to one side.
		if ch%4 == 0 || ch%4 == 3 {
			m.Pan[ch] = 64
		} else {
			m.Pan[ch] = 192
		}
	}
	return m, nil
}

func (r *reader) u16be() int { s := r.take(2); return int(s[0])<<8 | int(s[1]) }

// periodNote returns the note an Amiga period plays, 1 to 96, in
// FastTracker's notes: period 428 is C-4.
func periodNote(period int) uint8 {
	n := 48 + 12*math.Log2(428/float64(period))
	return uint8(min(max(int(math.Round(n)), 0), 95) + 1)
}
