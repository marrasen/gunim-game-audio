package tracker

import (
	"errors"
	"fmt"
)

// xmClock is FastTracker's Amiga clock: a sample's rate is it over the
// period, in its periods, 1712 for C-4.
const xmClock = 8363 * 1712

// readXM reads a FastTracker 2 module.
func readXM(b []byte) (*Module, error) {
	r := &reader{b: b, ok: true}
	r.take(17)
	m := &Module{Format: "XM", clock: xmClock}
	m.Title = r.str(20)
	r.take(1)
	m.Tracker = r.str(20)
	version := r.u16()
	headerAt := r.at
	headerSize := r.u32()
	songLen := r.u16()
	m.Restart = r.u16()
	m.Channels = r.u16()
	patterns := r.u16()
	instruments := r.u16()
	flags := r.u16()
	m.Speed, m.BPM = r.u16(), r.u16()
	orders := r.take(256)
	if !r.ok || m.Channels == 0 || m.Channels > 32 || songLen > 256 {
		return nil, fmt.Errorf("tracker: an XM of %d channels and %d orders", m.Channels, songLen)
	}
	if version < 0x0104 {
		return nil, fmt.Errorf("tracker: an XM of version %#x, older than FastTracker 2.04", version)
	}
	m.Linear = flags&1 != 0
	for _, o := range orders[:songLen] {
		m.Orders = append(m.Orders, int(o))
	}
	if m.Restart >= songLen {
		m.Restart = 0
	}
	if m.Speed == 0 {
		m.Speed = 6
	}
	if m.BPM == 0 {
		m.BPM = 125
	}
	r.at = headerAt + headerSize
	for range patterns {
		at := r.at
		hl := r.u32()
		r.u8()
		rows := r.u16()
		packed := r.u16()
		r.at = at + hl
		if rows == 0 || rows > 256 {
			rows = 64
		}
		p := Pattern{Rows: rows, Cells: make([]Cell, rows*m.Channels)}
		end := r.at + packed
		for i := 0; packed > 0 && i < len(p.Cells) && r.at < end; i++ {
			var c Cell
			first := r.u8()
			if first&0x80 != 0 {
				if first&1 != 0 {
					c.Note = uint8(r.u8())
				}
				if first&2 != 0 {
					c.Instrument = uint8(r.u8())
				}
				if first&4 != 0 {
					c.Volume = uint8(r.u8())
				}
				if first&8 != 0 {
					c.Effect = uint8(r.u8())
				}
				if first&16 != 0 {
					c.Param = uint8(r.u8())
				}
			} else {
				c.Note = uint8(first)
				c.Instrument, c.Volume, c.Effect, c.Param = uint8(r.u8()), uint8(r.u8()), uint8(r.u8()), uint8(r.u8())
			}
			if c.Note > KeyOff {
				c.Note = 0
			}
			p.Cells[i] = c
		}
		r.at = end
		m.Patterns = append(m.Patterns, p)
	}
	for range instruments {
		in, err := readXMInstrument(r)
		if err != nil {
			return nil, err
		}
		m.Instruments = append(m.Instruments, in)
	}
	if len(m.Patterns) == 0 {
		return nil, errors.New("tracker: an XM of no patterns")
	}
	// An order of a pattern there is not plays as an empty one.
	for _, o := range m.Orders {
		for o >= len(m.Patterns) {
			m.Patterns = append(m.Patterns, Pattern{Rows: 64, Cells: make([]Cell, 64*m.Channels)})
		}
	}
	m.Pan = make([]int, m.Channels)
	for i := range m.Pan {
		m.Pan[i] = 128
	}
	return m, nil
}

// readXMInstrument reads an instrument and its samples.
func readXMInstrument(r *reader) (Instrument, error) {
	at := r.at
	size := r.u32()
	in := Instrument{Name: r.str(22)}
	r.u8()
	n := r.u16()
	if n == 0 {
		r.at = at + max(size, 4)
		return in, nil
	}
	if n > 64 {
		return in, fmt.Errorf("tracker: an XM instrument of %d samples", n)
	}
	sampleHead := r.u32()
	for i := range in.Keymap {
		in.Keymap[i] = uint8(r.u8())
	}
	var vol, pan [12][2]int
	for i := range vol {
		vol[i] = [2]int{r.u16(), r.u16()}
	}
	for i := range pan {
		pan[i] = [2]int{r.u16(), r.u16()}
	}
	nv, np := r.u8(), r.u8()
	in.Vol.SustainAt, in.Vol.LoopStart, in.Vol.LoopEnd = r.u8(), r.u8(), r.u8()
	in.Pan.SustainAt, in.Pan.LoopStart, in.Pan.LoopEnd = r.u8(), r.u8(), r.u8()
	vt, pt := r.u8(), r.u8()
	in.VibType, in.VibSweep, in.VibDepth, in.VibRate = r.u8(), r.u8(), r.u8(), r.u8()
	in.Fadeout = r.u16()
	in.Vol.Points = append([][2]int(nil), vol[:min(nv, 12)]...)
	in.Pan.Points = append([][2]int(nil), pan[:min(np, 12)]...)
	in.Vol.On, in.Vol.Sustain, in.Vol.Loop = vt&1 != 0 && nv > 0, vt&2 != 0, vt&4 != 0
	in.Pan.On, in.Pan.Sustain, in.Pan.Loop = pt&1 != 0 && np > 0, pt&2 != 0, pt&4 != 0
	r.at = at + size
	type head struct{ length, loopStart, loopLen, kind int }
	heads := make([]head, n)
	for i := range n {
		sat := r.at
		h := head{length: r.u32(), loopStart: r.u32(), loopLen: r.u32()}
		s := Sample{Volume: min(r.u8(), 64), Finetune: int(int8(r.u8()))}
		h.kind = r.u8()
		s.Pan = r.u8()
		s.RelNote = int(int8(r.u8()))
		r.u8()
		s.Name = r.str(22)
		r.at = sat + max(sampleHead, 40)
		heads[i] = h
		in.Samples = append(in.Samples, s)
	}
	for i, h := range heads {
		s := &in.Samples[i]
		raw := r.take(min(h.length, len(r.b)-r.at))
		if h.kind&16 != 0 {
			// 16 bits, each the difference from the one before.
			s.Bits = 16
			s.Data = make([]float32, len(raw)/2)
			var acc int16
			for k := range s.Data {
				acc += int16(uint16(raw[2*k]) | uint16(raw[2*k+1])<<8)
				s.Data[k] = float32(acc) / 32768
			}
			h.loopStart /= 2
			h.loopLen /= 2
		} else {
			s.Bits = 8
			s.Data = make([]float32, len(raw))
			var acc int8
			for k, v := range raw {
				acc += int8(v)
				s.Data[k] = float32(acc) / 128
			}
		}
		if loop := h.kind & 3; loop != 0 && h.loopLen > 0 && h.loopStart < len(s.Data) {
			s.Loop = min(loop, 2)
			s.LoopStart = h.loopStart
			s.LoopLen = min(h.loopLen, len(s.Data)-h.loopStart)
		}
	}
	if !r.ok {
		return in, errors.New("tracker: an XM cut short in its instruments")
	}
	return in, nil
}
