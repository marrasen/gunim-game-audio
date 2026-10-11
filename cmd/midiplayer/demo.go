package main

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// demoSong is a MIDI file made in code, to try the player with nothing
// to drop: Neon Overture, 24 bars in A minor at 112 BPM, for a dozen
// instruments, so most of the stations have something to show. Four
// bars of strings, a pad and a glockenspiel; then drums, a bass, a
// piano and a saw lead; then a choir, brass, a guitar, a flute and a
// timpani on top; and bells to end.
func demoSong() []byte {
	const beat = 480
	const bar = 4 * beat
	type ev struct {
		tick, order int
		msg         []byte
	}
	var evs []ev
	add := func(tick int, msg ...byte) { evs = append(evs, ev{tick, len(evs), msg}) }
	note := func(ch byte, tick, dur int, key, vel int) {
		add(tick, 0x90|ch, byte(key), byte(vel))
		add(tick+max(dur, 10), 0x80|ch, byte(key), 0)
	}
	// The tempo, the metre, and each channel's instrument, level and
	// place.
	add(0, 0xFF, 0x51, 3, 0x08, 0x2D, 0x2B) // 535714 µs a beat, 112 BPM
	add(0, 0xFF, 0x58, 4, 4, 2, 24, 8)
	setup := []struct{ ch, prog, vol, pan, rev byte }{
		{0, 0, 92, 54, 50},   // piano
		{1, 38, 100, 64, 10}, // synth bass
		{2, 48, 78, 40, 80},  // strings
		{3, 81, 84, 70, 60},  // saw lead
		{4, 9, 70, 96, 70},   // glockenspiel
		{5, 52, 72, 30, 100}, // choir
		{6, 61, 80, 84, 50},  // brass
		{7, 25, 74, 100, 40}, // steel guitar
		{8, 73, 76, 24, 70},  // flute
		{10, 47, 90, 64, 60}, // timpani
		{11, 89, 64, 64, 90}, // warm pad
		{12, 14, 70, 80, 100},
	}
	for _, s := range setup {
		add(0, 0xC0|s.ch, s.prog)
		add(0, 0xB0|s.ch, 7, s.vol)
		add(0, 0xB0|s.ch, 10, s.pan)
		add(0, 0xB0|s.ch, 91, s.rev)
	}
	// A minor, F, C, G: each chord's root, and its notes from it.
	roots := []int{57, 53, 48, 55}
	triads := [][]int{{0, 3, 7}, {0, 4, 7}, {0, 4, 7}, {0, 4, 7}}
	// The lead's tune, two bars a line, in degrees of A minor over the
	// chords: 0 for a rest.
	aMinor := []int{57, 59, 60, 62, 64, 65, 67, 69, 71, 72, 74, 76, 77, 79, 81}
	tune := [][]int{
		{8, 0, 7, 8, 10, 0, 9, 8, 7, 0, 5, 7, 8, 0, 0, 0},
		{6, 0, 5, 6, 8, 0, 7, 6, 5, 0, 4, 5, 6, 0, 0, 0},
		{5, 0, 4, 5, 7, 0, 6, 5, 4, 0, 2, 4, 5, 0, 7, 0},
		{6, 0, 7, 8, 9, 0, 8, 7, 6, 0, 5, 4, 5, 0, 0, 0},
	}
	for b := range 24 {
		at := b * bar
		c := b % 4
		root := roots[c]
		chord := triads[c]
		intro, full, top, outro := b < 4, b >= 4 && b < 20, b >= 12 && b < 20, b >= 20
		// Strings and the pad hold the chord through every bar.
		for _, k := range chord {
			note(2, at, bar-20, root+k, 70)
			if !full || outro {
				note(11, at, bar-20, root+k-12, 60)
			}
		}
		// The glockenspiel climbs the chord in eighths.
		if intro || top {
			for i := range 8 {
				k := root + 12 + chord[i%3] + 12*(i/3%2)
				note(4, at+i*beat/2, beat/2, k, 70+i*4)
			}
		}
		if full {
			// Drums: a kick on each beat, the snare on 2 and 4, hats in
			// eighths, a crash each four bars, and a fill in each eighth.
			for i := range 4 {
				note(9, at+i*beat, 60, 36, 110)
				if i%2 == 1 {
					note(9, at+i*beat, 60, 38, 100)
				}
				note(9, at+i*beat, 30, 42, 80)
				note(9, at+i*beat+beat/2, 30, 42, 60)
			}
			if b%4 == 0 {
				note(9, at, 100, 49, 100)
			}
			if b%8 == 7 {
				for i, k := range []int{50, 48, 47, 45} {
					note(9, at+3*beat+i*beat/4, 60, k, 90+i*5)
				}
			}
			// The bass pulses the root in eighths, leaping the octave.
			for i := range 8 {
				k := root - 24
				if i%4 == 3 {
					k += 12
				}
				note(1, at+i*beat/2, beat/2-40, k, 96+(i%2)*14)
			}
			// The piano strikes the chord on 1 and the and of 2.
			for _, k := range chord {
				note(0, at, beat+beat/2-20, root+k, 84)
				note(0, at+beat+beat/2, 2*beat+beat/2-20, root+k+12, 72)
			}
			// The lead sings the tune.
			line := tune[(b-4)/2%4]
			half := (b % 2) * 8
			for i := range 8 {
				if d := line[half+i]; d != 0 {
					l := beat/2 - 30
					if i+1 < 8 && line[half+i+1] == 0 {
						l = beat - 30
					}
					note(3, at+i*beat/2, l, aMinor[d], 96)
				}
			}
		}
		if top {
			// The choir sings the chord; brass stabs on the offbeats; the
			// guitar picks; the flute answers the lead an octave up; the
			// timpani rolls into each four bars.
			for _, k := range chord {
				note(5, at, bar-40, root+k, 74)
				note(6, at+beat/2, beat/4, root+k+12, 92)
				note(6, at+2*beat+beat/2, beat/4, root+k+12, 88)
			}
			for i := range 8 {
				note(7, at+i*beat/2, beat, root+chord[[]int{0, 1, 2, 1}[i%4]]+(i/4)*12, 80)
			}
			for i, d := range []int{12, 11, 9, 8} {
				note(8, at+(4+i)*beat/2, beat/2-20, aMinor[d], 84)
			}
			if b%4 == 3 {
				for i := range 8 {
					note(10, at+3*beat+i*beat/8, beat/8, 45, 60+i*7)
				}
			}
		}
		if outro {
			note(0, at, bar, root+chord[2]+12, 70)
			note(12, at, bar, root+12, 80)
			note(12, at+2*beat, 2*beat, root+chord[1]+12, 70)
			if b == 23 {
				note(10, at, beat, 45, 110)
				note(9, at, 300, 49, 100)
			}
		}
	}
	end := 24*bar + bar
	add(end, 0xFF, 0x2F, 0)
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].tick != evs[j].tick {
			return evs[i].tick < evs[j].tick
		}
		// A note's end before a note's start at the same moment.
		oi, oj := evs[i].msg[0]&0xF0 == 0x80, evs[j].msg[0]&0xF0 == 0x80
		if oi != oj {
			return oi
		}
		return evs[i].order < evs[j].order
	})
	var trk []byte
	last := 0
	for _, e := range evs {
		trk = appendVarint(trk, e.tick-last)
		last = e.tick
		if e.msg[0] == 0xFF {
			trk = append(trk, e.msg[0], e.msg[1])
			trk = appendVarint(trk, len(e.msg)-3)
			trk = append(trk, e.msg[3:]...)
			continue
		}
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

// appendVarint appends v as a MIDI file's variable-length number.
func appendVarint(b []byte, v int) []byte {
	var tmp [4]byte
	n := 0
	for {
		tmp[n] = byte(v & 0x7F)
		n++
		v >>= 7
		if v == 0 {
			break
		}
	}
	for i := n - 1; i >= 0; i-- {
		c := tmp[i]
		if i > 0 {
			c |= 0x80
		}
		b = append(b, c)
	}
	return b
}
