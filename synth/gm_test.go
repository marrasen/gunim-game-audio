package synth

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"testing"

	"github.com/marrasen/gunim/audio"

	"github.com/marrasen/gunim-game-audio/midi"
)

func TestGMPatchesCompile(t *testing.T) {
	for prog := range 128 {
		if _, err := GMPatch(prog).compile(GMNames[prog]); err != nil {
			t.Errorf("program %d, %s: %v", prog, GMNames[prog], err)
		}
	}
	for _, kit := range []int{0, 24, 25} {
		if _, err := GMKit(kit).compile("kit"); err != nil {
			t.Errorf("kit %d: %v", kit, err)
		}
	}
}

// peak renders secs of g and returns its peak, failing on a sample that
// is not a number.
func peak(t *testing.T, g *GM, secs float64) float32 {
	t.Helper()
	buf := make([]float32, 2*512)
	var p float32
	for range int(secs * rate / 512) {
		g.Read(buf)
		for _, x := range buf {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				t.Fatalf("a sample of %v", x)
			}
			p = max(p, abs32(x))
		}
	}
	return p
}

func TestGMEveryInstrumentSounds(t *testing.T) {
	for prog := range 128 {
		g := NewGM()
		g.Send(midi.Program, byte(prog), 0, nil)
		g.Send(midi.NoteOn, 60, 100, nil)
		if p := peak(t, g, 0.5); p < 0.01 || p > 1 {
			t.Errorf("program %d, %s, peaks at %v", prog, GMNames[prog], p)
		}
	}
	for key := range 128 {
		g := NewGM()
		g.Send(midi.NoteOn|9, byte(key), 100, nil)
		p := peak(t, g, 0.3)
		if GMDrumName(key) != "" && p < 0.01 {
			t.Errorf("drum %d, %s, is silent", key, GMDrumName(key))
		}
		if GMDrumName(key) == "" && p > 1e-6 {
			t.Errorf("key %d, no drum, sounds", key)
		}
	}
}

func TestGMNoteOffAndSustain(t *testing.T) {
	g := NewGM()
	g.Send(midi.Program, 19, 0, nil) // an organ, which holds its notes
	g.Send(midi.NoteOn, 60, 100, nil)
	peak(t, g, 0.2)
	g.Send(midi.NoteOff, 60, 0, nil)
	peak(t, g, 1.5)
	if g.Sounding() {
		t.Fatal("a note let go still sounds")
	}
	g.Send(midi.Control, 64, 127, nil)
	g.Send(midi.NoteOn, 62, 100, nil)
	g.Send(midi.NoteOn, 62, 0, nil) // a note off, as a note on of velocity 0
	peak(t, g, 1.5)
	if !g.Sounding() {
		t.Fatal("the sustain pedal did not hold the note")
	}
	g.Send(midi.Control, 64, 0, nil)
	peak(t, g, 1.5)
	if g.Sounding() {
		t.Fatal("the note still sounds after the pedal's lift")
	}
}

func TestGMPitchBend(t *testing.T) {
	g := NewGM()
	g.Send(midi.NoteOn, 60, 100, nil)
	g.Send(midi.PitchBend, 0, 0x60, nil) // half way up: a tone at the range of 2
	peak(t, g, 0.05)
	v := g.ch[0].held[0].v
	if got := v.n.pitch; math.Abs(float64(got)-61) > 0.01 {
		t.Errorf("bent half way up, the note is at %v, want 61", got)
	}
	// The range set to 12 semitones by RPN 0.
	g.Send(midi.Control, 101, 0, nil)
	g.Send(midi.Control, 100, 0, nil)
	g.Send(midi.Control, 6, 12, nil)
	peak(t, g, 0.05)
	if got := v.n.pitch; math.Abs(float64(got)-66) > 0.01 {
		t.Errorf("with a range of 12, the note is at %v, want 66", got)
	}
}

func TestGMSysExDrumPart(t *testing.T) {
	g := NewGM()
	// GS: part 11, channel 11, to drums.
	g.Send(midi.SysEx, 0, 0, []byte{0x41, 0x10, 0x42, 0x12, 0x40, 0x1A, 0x15, 0x01, 0x10, 0xF7})
	if !g.ch[10].drums {
		t.Fatal("channel 11 is not a drum channel")
	}
	g.Send(midi.SysEx, 0, 0, []byte{0x7E, 0x7F, 0x09, 0x01, 0xF7})
	if g.ch[10].drums || !g.ch[9].drums {
		t.Fatal("GM reset did not restore channel 10 alone as drums")
	}
}

// demo is a MIDI file of 8 bars at 120 BPM: drums, a bass, piano chords,
// strings and a flute, for the player's tests, and to listen to.
func demo() []byte {
	type ev struct {
		tick int
		msg  []byte
	}
	var evs []ev
	add := func(tick int, msg ...byte) { evs = append(evs, ev{tick, msg}) }
	note := func(ch byte, tick, dur int, key, vel byte) {
		add(tick, 0x90|ch, key, vel)
		add(tick+dur, 0x80|ch, key, 0)
	}
	const beat = 480
	add(0, 0xC0, 0)  // piano
	add(0, 0xC1, 33) // finger bass
	add(0, 0xC2, 48) // strings
	add(0, 0xC3, 73) // flute
	add(0, 0xB2, 7, 70)
	add(0, 0xB0, 10, 40)
	add(0, 0xB3, 10, 88)
	chords := [][]byte{{57, 60, 64}, {53, 57, 60}, {48, 52, 55}, {55, 59, 62}}
	tune := []byte{76, 74, 72, 74, 76, 76, 76, 0, 74, 74, 74, 0, 76, 79, 79, 0}
	for bar := range 8 {
		at := bar * 4 * beat
		ch := chords[bar%4]
		for _, k := range ch {
			note(0, at, 2*beat-20, k, 80)
			note(0, at+2*beat, 2*beat-20, k, 70)
			note(2, at, 4*beat-10, k-12, 70)
		}
		for i := range 8 {
			note(1, at+i*beat/2, beat/2-30, ch[0]-24, byte(90+10*(i%2)))
		}
		for i := range 4 {
			note(9, at+i*beat, 100, map[bool]byte{true: 36, false: 38}[i%2 == 0], 110)
			note(9, at+i*beat+beat/2, 50, 42, 80)
			note(9, at+i*beat, 50, 42, 90)
		}
		if bar >= 4 {
			for i := range 4 {
				if k := tune[(bar-4)*4+i]; k != 0 {
					note(3, at+i*beat, beat-20, k, 90)
				}
			}
		}
	}
	add(8*4*beat, 0xFF, 0x2F, 0x00)
	// Sorted by tick, stably, so a note's end and the next start keep
	// their order.
	for i := 1; i < len(evs); i++ {
		for j := i; j > 0 && evs[j].tick < evs[j-1].tick; j-- {
			evs[j], evs[j-1] = evs[j-1], evs[j]
		}
	}
	var trk []byte
	last := 0
	for _, e := range evs {
		d := e.tick - last
		last = e.tick
		var vl []byte
		for vl = []byte{byte(d & 0x7F)}; d > 0x7F; {
			d >>= 7
			vl = append([]byte{byte(d&0x7F | 0x80)}, vl...)
		}
		trk = append(trk, vl...)
		trk = append(trk, e.msg...)
	}
	var b bytes.Buffer
	b.WriteString("MThd")
	b.Write(binary.BigEndian.AppendUint32(nil, 6))
	b.Write([]byte{0, 0, 0, 1, beat >> 8, beat & 0xFF})
	b.WriteString("MTrk")
	b.Write(binary.BigEndian.AppendUint32(nil, uint32(len(trk))))
	b.Write(trk)
	return b.Bytes()
}

func TestMIDIPlayerPlaysToTheEnd(t *testing.T) {
	data := demo()
	if out := os.Getenv("GM_DEMO"); out != "" {
		// GM_DEMO=demo.mid go test -run MIDIPlayer writes the demo out.
		if err := os.WriteFile(out, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := midi.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Length(); math.Abs(got-16) > 1e-9 {
		t.Fatalf("Length = %v, want 16 s", got)
	}
	p := NewMIDIPlayer(f)
	buf := make([]float32, 2*1000)
	var m audio.LoudnessMeter
	frames := 0
	for {
		n, err := p.Read(buf)
		m.Write(buf[:2*n])
		frames += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if frames > 30*rate {
			t.Fatal("still playing at 30 s")
		}
	}
	// The file, then the last notes' release and the reverb's tail.
	if secs := float64(frames) / rate; secs < 16 || secs > 21 {
		t.Errorf("played %.2f s, want 16 and a tail", secs)
	}
	lufs, _ := m.Integrated()
	if lufs < -22 || lufs > -12 || m.Peak() > 1 {
		t.Errorf("played at %.1f LUFS, peak %v", lufs, m.Peak())
	}
	t.Logf("%.1f LUFS, peak %.1f dB", lufs, 20*math.Log10(float64(m.Peak())))

	// Sought to bar 5, the flute is set for the tune.
	p.Seek(8.5)
	if got := p.gm.ch[3].program; got != 73 {
		t.Errorf("sought, channel 4 plays program %d, want 73", got)
	}
	if got := p.Position(); got != 8.5 {
		t.Errorf("sought to 8.5 s, Position = %v", got)
	}
}
