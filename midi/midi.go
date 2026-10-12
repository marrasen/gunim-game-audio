// Package midi reads Standard MIDI Files, the .mid files a sequencer
// saves, into events timed in seconds, for package synth's General MIDI
// player to play.
//
//	f, err := midi.Read(file)
//	for _, e := range f.Events() {
//		fmt.Println(e.Time, e.Channel(), e.Kind())
//	}
//
// It reads formats 0, 1 and 2, timed in ticks a beat or in SMPTE
// frames, with running status, tempo changes and system exclusive
// messages.
package midi

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
)

// The kinds of channel message, a status byte's top four bits.
const (
	NoteOff       = 0x80
	NoteOn        = 0x90
	KeyPressure   = 0xA0
	Control       = 0xB0
	Program       = 0xC0
	ChanPressure  = 0xD0
	PitchBend     = 0xE0
	SysEx         = 0xF0
	Meta          = 0xFF
	metaTempo     = 0x51
	metaEndTrack  = 0x2F
	metaTrackName = 0x03
	metaText      = 0x01
	metaCopyright = 0x02
)

// An Event is a message of a track, at a time.
type Event struct {
	// Tick is when it comes in the file's ticks, and Time in seconds
	// from the start, the tempo map applied.
	Tick int64
	Time float64
	// Track is the track it came from, from 0.
	Track int
	// Status is the message's status byte: a channel message's kind and
	// channel, SysEx for a system exclusive message, or Meta.
	Status byte
	// Data1 and Data2 are a channel message's data bytes; a meta event's
	// type is Data1.
	Data1, Data2 byte
	// Data is a meta event's or a system exclusive message's bytes.
	Data []byte
}

// Kind returns a channel message's kind, as NoteOn, or SysEx or Meta.
func (e Event) Kind() byte {
	if e.Status >= 0xF0 {
		return e.Status
	}
	return e.Status & 0xF0
}

// Channel returns a channel message's channel, from 0 to 15.
func (e Event) Channel() int { return int(e.Status & 0x0F) }

// Tempo returns the microseconds a beat a tempo event sets, and whether
// e is one.
func (e Event) Tempo() (int, bool) {
	if e.Status != Meta || e.Data1 != metaTempo || len(e.Data) < 3 {
		return 0, false
	}
	return int(e.Data[0])<<16 | int(e.Data[1])<<8 | int(e.Data[2]), true
}

// A File is a Standard MIDI File, read.
type File struct {
	// Format is 0, one track; 1, tracks played together; or 2, tracks
	// each a song of its own, which Events plays one after another.
	Format int
	// Division is the ticks a beat, or, where negative, the file is
	// timed in SMPTE frames: its high byte the negated frames a second
	// and its low the ticks a frame.
	Division int
	// Tracks are each track's events, in order, each timed.
	Tracks [][]Event
}

// Read reads a Standard MIDI File from r.
func Read(r io.Reader) (*File, error) {
	br := bufio.NewReader(r)
	f := &File{}
	var ntracks int
	for found := false; !found || len(f.Tracks) < ntracks; {
		id, body, err := chunk(br)
		if err == io.EOF && found {
			// Some files say they have more tracks than they hold.
			break
		}
		if err != nil {
			return nil, fmt.Errorf("midi: %w", err)
		}
		switch {
		case !found:
			if id != "MThd" || len(body) < 6 {
				return nil, errors.New("midi: not a Standard MIDI File")
			}
			found = true
			f.Format = int(binary.BigEndian.Uint16(body[0:]))
			ntracks = int(binary.BigEndian.Uint16(body[2:]))
			f.Division = int(int16(binary.BigEndian.Uint16(body[4:])))
			if f.Division == 0 {
				return nil, errors.New("midi: the file's division is 0")
			}
		case id == "MTrk":
			t, err := track(body, len(f.Tracks))
			if err != nil {
				return nil, fmt.Errorf("midi: track %d: %w", len(f.Tracks)+1, err)
			}
			f.Tracks = append(f.Tracks, t)
		}
	}
	f.time()
	return f, nil
}

// chunk reads a chunk: its type and its body.
func chunk(r io.Reader) (string, []byte, error) {
	var head [8]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return "", nil, err
	}
	n := binary.BigEndian.Uint32(head[4:])
	if n > 1<<28 {
		return "", nil, fmt.Errorf("a chunk of %d bytes is too big", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return "", nil, fmt.Errorf("a chunk cut short: %w", err)
	}
	return string(head[:4]), body, nil
}

// track reads a track chunk's events, timed in ticks.
func track(b []byte, index int) ([]Event, error) {
	var (
		events  []Event
		tick    int64
		running byte
	)
	at := 0
	varint := func() (int, error) {
		v := 0
		for i := 0; i < 4; i++ {
			if at >= len(b) {
				return 0, io.ErrUnexpectedEOF
			}
			c := b[at]
			at++
			v = v<<7 | int(c&0x7F)
			if c&0x80 == 0 {
				return v, nil
			}
		}
		return 0, errors.New("a number longer than 4 bytes")
	}
	for at < len(b) {
		dt, err := varint()
		if err != nil {
			return nil, err
		}
		tick += int64(dt)
		if at >= len(b) {
			return nil, io.ErrUnexpectedEOF
		}
		e := Event{Tick: tick, Track: index}
		status := b[at]
		switch {
		case status == Meta:
			if at+2 > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			e.Status, e.Data1 = Meta, b[at+1]
			at += 2
			n, err := varint()
			if err != nil {
				return nil, err
			}
			if at+n > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			e.Data = b[at : at+n]
			at += n
			if e.Data1 == metaEndTrack {
				// The end of the track is kept, as the time a file
				// rests to before it ends, or loops.
				return append(events, e), nil
			}
		case status == 0xF0 || status == 0xF7:
			at++
			n, err := varint()
			if err != nil {
				return nil, err
			}
			if at+n > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			e.Status, e.Data = SysEx, b[at:at+n]
			at += n
			// A system exclusive message cancels running status.
			running = 0
		default:
			if status&0x80 != 0 {
				running = status
				at++
			} else if running == 0 {
				return nil, fmt.Errorf("a data byte %#x with no status before it", status)
			}
			e.Status = running
			need := 2
			if k := running & 0xF0; k == Program || k == ChanPressure {
				need = 1
			}
			if at+need > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			e.Data1 = b[at] & 0x7F
			if need == 2 {
				e.Data2 = b[at+1] & 0x7F
			}
			at += need
		}
		events = append(events, e)
	}
	return events, nil
}

// time times every event in seconds. Format 0 and 1 files share one
// tempo map, the tempo events of every track; a format 2 file's tracks
// keep their own.
func (f *File) time() {
	if f.Format == 2 {
		for _, t := range f.Tracks {
			f.timeTrack([][]Event{t})
		}
		return
	}
	f.timeTrack(f.Tracks)
}

// timeTrack times tracks by the tempo events among them.
func (f *File) timeTrack(tracks [][]Event) {
	if f.Division < 0 {
		// SMPTE: frames a second times ticks a frame, whatever the tempo.
		fps := float64(-int8(f.Division >> 8))
		if fps == 29 {
			fps = 29.97
		}
		perSec := fps * float64(f.Division&0xFF)
		for _, t := range tracks {
			for i := range t {
				t[i].Time = float64(t[i].Tick) / perSec
			}
		}
		return
	}
	type change struct {
		tick int64
		us   int
	}
	var tempos []change
	for _, t := range tracks {
		for _, e := range t {
			if us, ok := e.Tempo(); ok && us > 0 {
				tempos = append(tempos, change{e.Tick, us})
			}
		}
	}
	slices.SortStableFunc(tempos, func(a, b change) int { return int(a.tick - b.tick) })
	div := float64(f.Division)
	for _, t := range tracks {
		// Walk the tempo map along the track, which is in tick order.
		ti, at, sec, us := 0, int64(0), 0.0, 500000
		for i := range t {
			for ti < len(tempos) && tempos[ti].tick <= t[i].Tick {
				sec += float64(tempos[ti].tick-at) * float64(us) / 1e6 / div
				at, us = tempos[ti].tick, tempos[ti].us
				ti++
			}
			t[i].Time = sec + float64(t[i].Tick-at)*float64(us)/1e6/div
		}
	}
}

// Events returns every track's events in one list, in time order. A
// format 2 file's tracks play one after another.
func (f *File) Events() []Event {
	var all []Event
	offset := 0.0
	for _, t := range f.Tracks {
		end := 0.0
		for _, e := range t {
			if f.Format == 2 {
				e.Time += offset
			}
			all = append(all, e)
			end = max(end, e.Time)
		}
		if f.Format == 2 {
			offset = end
		}
	}
	// Stable, so events at the same moment keep the order of their
	// tracks and, within a track, the order written: a program change
	// before the note it is for.
	slices.SortStableFunc(all, func(a, b Event) int {
		switch {
		case a.Time < b.Time:
			return -1
		case a.Time > b.Time:
			return 1
		}
		return 0
	})
	return all
}

// Length returns how long the file plays, in seconds: to its last
// event.
func (f *File) Length() float64 {
	evs := f.Events()
	if len(evs) == 0 {
		return 0
	}
	return evs[len(evs)-1].Time
}

// Name returns the file's name, the first track's name, or else its
// first text.
func (f *File) Name() string {
	text := ""
	for _, t := range f.Tracks[:min(len(f.Tracks), 1)] {
		for _, e := range t {
			if e.Status != Meta {
				continue
			}
			if e.Data1 == metaTrackName && len(e.Data) > 0 {
				return string(e.Data)
			}
			if e.Data1 == metaText && text == "" {
				text = string(e.Data)
			}
		}
	}
	return text
}
