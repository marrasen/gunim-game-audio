package midi

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// smf builds a Standard MIDI File of format, division and tracks, each
// track's bytes its events, written.
func smf(format, division int, tracks ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("MThd")
	b.Write(binary.BigEndian.AppendUint32(nil, 6))
	for _, v := range []int{format, len(tracks), division} {
		b.Write(binary.BigEndian.AppendUint16(nil, uint16(v)))
	}
	for _, t := range tracks {
		b.WriteString("MTrk")
		b.Write(binary.BigEndian.AppendUint32(nil, uint32(len(t))))
		b.Write(t)
	}
	return b.Bytes()
}

func TestReadTimesByTempo(t *testing.T) {
	// Track 1 sets 120 BPM, then 60 BPM at beat 2; track 2 plays notes,
	// in running status, at beats 0, 1, 3 and 4. 480 ticks a beat; 960 is
	// 0x87 0x40 as a variable-length number.
	tempo := []byte{
		0x00, 0xFF, 0x51, 0x03, 0x07, 0xA1, 0x20, // 500000 us
		0x87, 0x40, 0xFF, 0x51, 0x03, 0x0F, 0x42, 0x40, // 1000000 us, at 960
		0x00, 0xFF, 0x2F, 0x00,
	}
	notes := []byte{
		0x00, 0xFF, 0x03, 0x04, 'T', 'e', 's', 't',
		0x00, 0x90, 60, 100, // tick 0
		0x83, 0x60, 60, 0, // tick 480, running status
		0x87, 0x40, 0xC0, 5, // tick 1440, a program change
		0x00, 0x90, 64, 90,
		0x83, 0x60, 0x80, 64, 0, // tick 1920
		0x00, 0xFF, 0x2F, 0x00,
	}
	f, err := Read(bytes.NewReader(smf(1, 480, tempo, notes)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != 1 || f.Division != 480 || len(f.Tracks) != 2 {
		t.Fatalf("format %d, division %d, %d tracks", f.Format, f.Division, len(f.Tracks))
	}
	if got := f.Name(); got != "" {
		// The first track is the tempo track, which is unnamed.
		t.Errorf("Name = %q, want none", got)
	}
	var times []float64
	var kinds []byte
	for _, e := range f.Tracks[1] {
		if e.Status != Meta {
			times = append(times, e.Time)
			kinds = append(kinds, e.Kind())
		}
	}
	// Beats 0 and 1 at 120 BPM; beat 3 is 1 s, then a beat at 60 BPM.
	want := []float64{0, 0.5, 2, 2, 3}
	wantKinds := []byte{NoteOn, NoteOn, Program, NoteOn, NoteOff}
	if len(times) != len(want) {
		t.Fatalf("got %d events, want %d", len(times), len(want))
	}
	for i := range want {
		if math.Abs(times[i]-want[i]) > 1e-9 || kinds[i] != wantKinds[i] {
			t.Errorf("event %d: %#x at %g s, want %#x at %g s", i, kinds[i], times[i], wantKinds[i], want[i])
		}
	}
	if got := f.Length(); math.Abs(got-3) > 1e-9 {
		t.Errorf("Length = %g, want 3", got)
	}
	evs := f.Events()
	for i := 1; i < len(evs); i++ {
		if evs[i].Time < evs[i-1].Time {
			t.Fatalf("Events out of order at %d", i)
		}
	}
}

func TestReadSMPTE(t *testing.T) {
	// 25 frames a second, 40 ticks a frame: 1000 ticks a second.
	div := 0xE728                                                      // -25 in the high byte, 40 in the low
	track := []byte{0x87, 0x68, 0x90, 60, 100, 0x00, 0xFF, 0x2F, 0x00} // tick 1000
	f, err := Read(bytes.NewReader(smf(0, div, track)))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Tracks[0][0].Time; math.Abs(got-1) > 1e-9 {
		t.Errorf("a note at tick 1000 comes at %g s, want 1", got)
	}
}

func TestReadRejects(t *testing.T) {
	for name, b := range map[string][]byte{
		"not midi":   []byte("RIFF....WAVEfmt "),
		"cut short":  smf(0, 96, []byte{0x00, 0x90, 60})[:30],
		"no status":  smf(0, 96, []byte{0x00, 60, 100}),
		"zero ticks": smf(0, 0, []byte{0x00, 0xFF, 0x2F, 0x00}),
	} {
		if _, err := Read(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: read with no error", name)
		}
	}
}
