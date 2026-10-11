package tracker

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// modCell is a cell of a MOD as the tests write it.
type modCell struct {
	period, sample, effect, param int
}

// buildMOD builds a 4-channel MOD of one sample, a square wave of 32
// frames, looped, and the patterns given, each 64 rows of 4 cells,
// played in order.
func buildMOD(patterns [][64][4]modCell, orders []int) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 20))
	for i := range 31 {
		h := make([]byte, 30)
		if i == 0 {
			copy(h, "square")
			binary.BigEndian.PutUint16(h[22:], 16) // 32 bytes
			h[25] = 64
			binary.BigEndian.PutUint16(h[26:], 0)
			binary.BigEndian.PutUint16(h[28:], 16)
		}
		b.Write(h)
	}
	b.WriteByte(byte(len(orders)))
	b.WriteByte(127)
	o := make([]byte, 128)
	for i, v := range orders {
		o[i] = byte(v)
	}
	b.Write(o)
	b.WriteString("M.K.")
	for _, p := range patterns {
		for _, row := range p {
			for _, c := range row {
				b.Write([]byte{byte(c.sample&0xF0 | c.period>>8), byte(c.period), byte(c.sample<<4&0xF0 | c.effect), byte(c.param)})
			}
		}
	}
	for i := range 32 {
		if i < 16 {
			b.WriteByte(100)
		} else {
			b.WriteByte(byte(256 - 100))
		}
	}
	return b.Bytes()
}

// pitch returns the frequency of the loudest channel of p's next secs of
// sound, by its rising zero crossings.
func pitch(t *testing.T, p *Player, secs float64) float64 {
	t.Helper()
	n := int(secs * rate)
	buf := make([]float32, 2*n)
	got, _ := p.Read(buf)
	crossings, first, last := 0, -1, -1
	for i := 1; i < got; i++ {
		a, b := buf[2*(i-1)]+buf[2*(i-1)+1], buf[2*i]+buf[2*i+1]
		if a <= 0 && b > 0 {
			if first < 0 {
				first = i
			}
			last = i
			crossings++
		}
	}
	if crossings < 2 {
		return 0
	}
	return float64(crossings-1) * rate / float64(last-first)
}

func TestAMODNotePlaysAtItsPeriod(t *testing.T) {
	var p [64][4]modCell
	p[0][0] = modCell{period: 428, sample: 1}
	m, err := Read(bytes.NewReader(buildMOD([][64][4]modCell{p}, []int{0})))
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != "MOD" || m.Channels != 4 || NoteName(m.Patterns[0].Cells[0].Note, m.OctaveShift) != "C-2" {
		t.Fatalf("read %s, %d channels, a note of %s", m.Format, m.Channels, NoteName(m.Patterns[0].Cells[0].Note, m.OctaveShift))
	}
	pl := NewPlayer(m)
	// Period 428 plays the sample at 7093789.2/856 Hz: its 32 frames a
	// cycle make 259 Hz.
	if f := pitch(t, pl, 0.5); math.Abs(f-8287.1/32) > 2 {
		t.Errorf("C-2 plays at %.1f Hz, want 259", f)
	}
}

func TestTheSongFlowsAsItsEffectsSay(t *testing.T) {
	// Pattern 0: speed 3 on row 0, a break to row 16 of the next order
	// at row 8. Pattern 1: from row 16, a jump back to order 0 at row
	// 20, which ends the song.
	var p0, p1 [64][4]modCell
	p0[0][0] = modCell{period: 428, sample: 1, effect: 0xF, param: 3}
	p0[8][1] = modCell{effect: 0xD, param: 0x16}
	p1[20][2] = modCell{effect: 0xB, param: 0}
	m, err := Read(bytes.NewReader(buildMOD([][64][4]modCell{p0, p1}, []int{0, 1})))
	if err != nil {
		t.Fatal(err)
	}
	tl := Scan(m)
	if len(tl.Rows) != 9+5 {
		t.Fatalf("the song plays %d rows, want 14", len(tl.Rows))
	}
	if r := tl.Rows[9]; r.Order != 1 || r.Row != 16 {
		t.Errorf("after the break, order %d row %d plays, want order 1 row 16", r.Order, r.Row)
	}
	// 14 rows of 3 ticks at 125 BPM, 0.02 s a tick.
	if math.Abs(tl.Length-14*3*0.02) > 1e-6 {
		t.Errorf("the song lasts %.4f s, want 0.84", tl.Length)
	}
	if len(tl.Notes) != 1 || tl.Notes[0].Ch != 0 || tl.Notes[0].Time != 0 {
		t.Errorf("the notes are %+v", tl.Notes)
	}
}

func TestAPatternLoopPlaysItsRowsAgain(t *testing.T) {
	// Rows 0 to 3 play three times: E60 at row 0, E62 at row 3. Then a
	// jump ends the song at row 4.
	var p [64][4]modCell
	p[0][0] = modCell{effect: 0xE, param: 0x60}
	p[3][0] = modCell{effect: 0xE, param: 0x62}
	p[4][1] = modCell{effect: 0xB, param: 0}
	m, _ := Read(bytes.NewReader(buildMOD([][64][4]modCell{p}, []int{0})))
	if got := len(Scan(m).Rows); got != 4*3+1 {
		t.Errorf("the loop plays %d rows, want 13", got)
	}
}

func TestVolumeSlidesAndSeek(t *testing.T) {
	var p [64][4]modCell
	p[0][0] = modCell{period: 428, sample: 1}
	for r := 1; r < 64; r++ {
		p[r][0] = modCell{effect: 0xA, param: 0x01}
	}
	m, _ := Read(bytes.NewReader(buildMOD([][64][4]modCell{p}, []int{0})))
	pl := NewPlayer(m)
	level := func() float32 {
		buf := make([]float32, 2*2400)
		pl.Read(buf)
		var peak float32
		for _, x := range buf {
			peak = max(peak, float32(math.Abs(float64(x))))
		}
		return peak
	}
	a := level()
	pl.Seek(4) // 33 rows in, the volume slid down 5 a row
	if math.Abs(pl.TimeAt(pl.out)-4) > 0.01 {
		t.Errorf("sought to 4 s, TimeAt says %v", pl.TimeAt(pl.out))
	}
	level() // what was in flight before the seek: the limiter's look ahead
	b := level()
	if !(b < a*0.2) {
		t.Errorf("a volume slide down left the level at %v, from %v", b, a)
	}
}

// buildXM builds an XM of 2 channels, one instrument of one 16-bit
// sample, a square wave of 32 frames, looped, with a volume envelope
// that falls from 64 to 0 over 10 ticks once let go, and one pattern of
// the cells given.
func buildXM(cells [][2]Cell, linear bool) []byte {
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("Extended Module: ")
	b.Write(append([]byte("test song"), make([]byte, 11)...))
	b.WriteByte(0x1A)
	b.Write(append([]byte("FastTracker v2.00"), make([]byte, 3)...))
	le(uint16(0x0104))
	le(uint32(276))
	le(uint16(1)) // song length
	le(uint16(0)) // restart
	le(uint16(2)) // channels
	le(uint16(1)) // patterns
	le(uint16(1)) // instruments
	le(uint16(map[bool]uint16{true: 1, false: 0}[linear]))
	le(uint16(6))
	le(uint16(125))
	b.Write(make([]byte, 256))
	var pat bytes.Buffer
	for _, row := range cells {
		for _, c := range row {
			pat.Write([]byte{c.Note, c.Instrument, c.Volume, c.Effect, c.Param})
		}
	}
	le(uint32(9))
	b.WriteByte(0)
	le(uint16(len(cells)))
	le(uint16(pat.Len()))
	b.Write(pat.Bytes())
	// The instrument.
	le(uint32(263))
	b.Write(append([]byte("square"), make([]byte, 16)...))
	b.WriteByte(0)
	le(uint16(1))
	le(uint32(40))
	b.Write(make([]byte, 96))
	env := [12][2]uint16{{0, 64}, {2, 64}, {12, 0}}
	for _, pt := range env {
		le(pt)
	}
	b.Write(make([]byte, 48))
	b.Write([]byte{3, 0, 1, 0, 0, 0, 0, 0, 1 | 2, 0, 0, 0, 0, 0})
	le(uint16(0))
	le(uint16(0))
	b.Write(make([]byte, 263-243))
	// The sample's header and its data, 16 bits, each the difference
	// from the one before.
	le(uint32(64))
	le(uint32(0))
	le(uint32(64))
	b.WriteByte(64)
	b.WriteByte(0)
	b.WriteByte(1 | 16)
	b.WriteByte(128)
	b.WriteByte(0)
	b.WriteByte(0)
	b.Write(make([]byte, 22))
	prev := int16(0)
	for i := range 32 {
		v := int16(12000)
		if i >= 16 {
			v = -12000
		}
		le(v - prev)
		prev = v
	}
	return b.Bytes()
}

func TestAnXMPlaysItsNotesAndEnvelope(t *testing.T) {
	cells := make([][2]Cell, 16)
	cells[0][0] = Cell{Note: 49, Instrument: 1} // C-4
	cells[8][0] = Cell{Note: KeyOff}
	for _, linear := range []bool{true, false} {
		m, err := Read(bytes.NewReader(buildXM(cells, linear)))
		if err != nil {
			t.Fatal(err)
		}
		if m.Format != "XM" || m.Title != "test song" || len(m.Instruments) != 1 || m.Instruments[0].Samples[0].Bits != 16 {
			t.Fatalf("read %+v", m)
		}
		p := NewPlayer(m)
		// C-4 plays its sample at 8363 Hz: 261 Hz.
		if f := pitch(t, p, 0.4); math.Abs(f-8363.0/32) > 2 {
			t.Errorf("linear %v: C-4 plays at %.1f Hz, want 261", linear, f)
		}
		// Let go at row 8, 0.96 s in, the envelope falls to silence in 10
		// ticks: by 1.3 s nothing sounds.
		p.Seek(1.3)
		buf := make([]float32, 2*2400)
		p.TakeMeters()
		p.Read(buf)
		// The channel's own meter: the reverb's tail rings on in the mix.
		if lvl := p.TakeMeters().Channels[0]; lvl > 0.001 {
			t.Errorf("linear %v: let go, the note still sounds at %v", linear, lvl)
		}
	}
}

func TestNotesAreNamedAsTheirTrackersShowThem(t *testing.T) {
	for _, c := range []struct {
		note  uint8
		shift int
		want  string
	}{{49, 0, "C-4"}, {50, 0, "C#4"}, {49, -2, "C-2"}, {KeyOff, 0, "==="}, {0, 0, "---"}} {
		if got := NoteName(c.note, c.shift); got != c.want {
			t.Errorf("NoteName(%d, %d) = %s, want %s", c.note, c.shift, got, c.want)
		}
	}
}

func TestAnOldMODIsTimedByTheVerticalBlank(t *testing.T) {
	// F40 then F05: by the CIA, BPM 64 at speed 5, which plays the 50
	// orders for over 10 minutes; by the vertical blank, speed 64 for a
	// row and then speed 5 at 125, which plays them in half the time.
	var p [64][4]modCell
	p[0][0] = modCell{period: 428, sample: 1, effect: 0xF, param: 0x40}
	p[1][0] = modCell{effect: 0xF, param: 5}
	orders := make([]int, 50)
	m, err := Read(bytes.NewReader(buildMOD([][64][4]modCell{p}, orders)))
	if err != nil {
		t.Fatal(err)
	}
	if !m.VBlank {
		t.Fatal("the module is timed by the CIA")
	}
	want := 50 * (64*0.02 + 63*5*0.02)
	if got := Scan(m).Length; math.Abs(got-want) > 0.01 {
		t.Errorf("by the vertical blank, the song lasts %.2f s, want %.2f", got, want)
	}
	// One short enough by the CIA stays so.
	m2, _ := Read(bytes.NewReader(buildMOD([][64][4]modCell{p}, orders[:4])))
	if m2.VBlank {
		t.Error("a short module is timed by the vertical blank")
	}
}
