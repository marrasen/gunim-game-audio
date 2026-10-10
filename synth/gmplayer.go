package synth

import (
	"io"
	"math"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim-game-audio/midi"
)

// GM is a General MIDI synthesizer: sixteen channels, each playing one
// of General MIDI's 128 instruments, made as this engine's patches, or,
// on channel 10, its drum kit, through a reverb and a chorus. It plays
// the messages it is sent, as a sound module plays what comes down its
// MIDI cable, and is an audio source a mixer plays.
//
// It is not safe for use by more than one goroutine at once; a
// [MIDIPlayer] sends it a file's messages as it plays.
type GM struct {
	ch [16]gmChannel
	// patches are the compiled instruments, by program, and kits the
	// drum kits, by key, by the kit's program: each made once.
	patches map[int]*patch
	kits    map[int]*[128]*drum
	ctx     renderCtx
	rev     *reverb
	cho     *chorus
	lim     *limiter
	// Gain is the whole mix's level, before its limiter.
	Gain float32
	// The buffers: a channel's sound, the mix, and the sends to the
	// reverb and the chorus.
	tl, tr, ml, mr, rl, rr, cl, cr, dl, dr []float32
}

// gmChannel is a channel's state: what it plays and how its controllers
// stand.
type gmChannel struct {
	program int
	bank    byte
	drums   bool
	p       *patch
	kit     *[128]*drum
	vs      *voices
	hs      *hits
	held    []gmHeld
	// vol, expr, rev and cho are from 0 to 1, pan from -1 to 1.
	vol, expr, pan, rev, cho float32
	sustain                  bool
	// bend is the bend wheel, from -1 to 1, and bendRange how many
	// semitones it bends at its end; mod is the modulation wheel, from 0
	// to 1, which shakes the pitch by vibrato, its phase vibPh.
	bend, bendRange float32
	mod, vibPh      float32
	// shift is how many semitones the bend and the vibrato move the
	// channel's notes now, as last given them.
	shift float32
	// rpn is the registered parameter the data entry sets, MSB and LSB;
	// 127 127 is none.
	rpn [2]byte
}

// gmHeld is a note a channel holds, by its key, on the voice that plays
// it, until its note off, or its sustain pedal's lift.
type gmHeld struct {
	key       int
	v         *voice
	age       uint64
	sustained bool
}

// NewGM returns a General MIDI synthesizer, reset.
func NewGM() *GM {
	g := &GM{
		patches: map[int]*patch{},
		kits:    map[int]*[128]*drum{},
		ctx:     renderCtx{beatHz: 2, l: make([]float32, control), r: make([]float32, control)},
		rev:     newReverb(),
		cho:     newChorus(chorusJuno12),
		lim:     newLimiter(-1),
		Gain:    0.6,
	}
	g.cho.mix = 1
	g.rev.set(0.9, 2.2, 0.5, 0.02)
	for _, b := range []*[]float32{&g.tl, &g.tr, &g.ml, &g.mr, &g.rl, &g.rr, &g.cl, &g.cr, &g.dl, &g.dr} {
		*b = make([]float32, maxBlock)
	}
	for i := range g.ch {
		c := &g.ch[i]
		c.vs = newVoices(32, uint64(i)*104729+1)
		c.hs = newHits(24)
	}
	g.Reset()
	return g
}

// Reset resets every channel as General MIDI's reset does: each plays
// the piano, channel 10 the drums, and their controllers are reset.
// Notes sounding are let go.
func (g *GM) Reset() {
	for i := range g.ch {
		c := &g.ch[i]
		g.releaseAll(c)
		c.bank, c.drums = 0, i == 9
		g.setProgram(c, 0)
		c.vol, c.pan, c.rev, c.cho = 100.0/127, 0, 40.0/127, 0
		g.resetControllers(c)
	}
}

// resetControllers resets what General MIDI's Reset All Controllers
// does.
func (g *GM) resetControllers(c *gmChannel) {
	c.expr, c.bend, c.bendRange, c.mod = 1, 0, 2, 0
	c.rpn = [2]byte{127, 127}
	if c.sustain {
		c.sustain = false
		g.lift(c)
	}
}

// Send plays a MIDI message: a channel message, status and its data, or
// a system exclusive message, its bytes after the 0xF0 in data.
func (g *GM) Send(status, data1, data2 byte, data []byte) {
	if status == midi.SysEx {
		g.sysex(data)
		return
	}
	if status < 0x80 || status >= 0xF0 {
		return
	}
	c := &g.ch[status&0x0F]
	switch status & 0xF0 {
	case midi.NoteOn:
		if data2 == 0 {
			g.noteOff(c, int(data1))
		} else {
			g.noteOn(c, int(data1), data2)
		}
	case midi.NoteOff:
		g.noteOff(c, int(data1))
	case midi.Control:
		g.control(c, data1, data2)
	case midi.Program:
		g.setProgram(c, int(data1))
	case midi.PitchBend:
		c.bend = float32(int(data2)<<7|int(data1)-8192) / 8192
	}
}

// Event plays e, a message of a MIDI file; it plays no meta event.
func (g *GM) Event(e midi.Event) {
	if e.Status != midi.Meta {
		g.Send(e.Status, e.Data1, e.Data2, e.Data)
	}
}

// velocity is how loud a note of MIDI velocity v plays: General MIDI's
// level falls as velocity's square does.
func velocity(v byte) float32 {
	x := float32(v) / 127
	return x*x*0.7 + x*0.3
}

func (g *GM) noteOn(c *gmChannel, key int, vel byte) {
	if c.drums {
		if d := c.kit[key&127]; d != nil {
			c.hs.play(*d, velocity(vel), 0, float32(key), rate/2, 0)
		}
		return
	}
	n := note{pitch: float32(key) + c.shift, vel: velocity(vel), res: -1}
	v := c.vs.play(c.p, n)
	c.held = append(c.held, gmHeld{key: key, v: v, age: v.age})
}

func (g *GM) noteOff(c *gmChannel, key int) {
	for i, h := range c.held {
		if h.key != key || h.sustained {
			continue
		}
		if c.sustain {
			c.held[i].sustained = true
			return
		}
		g.release(h)
		c.held = append(c.held[:i], c.held[i+1:]...)
		return
	}
}

// release lets go of h's note, unless its voice has been taken for
// another.
func (g *GM) release(h gmHeld) {
	if h.v.on && h.v.age == h.age {
		h.v.release()
	}
}

// lift lets go of the notes the sustain pedal held.
func (g *GM) lift(c *gmChannel) {
	held := c.held[:0]
	for _, h := range c.held {
		if h.sustained {
			g.release(h)
		} else {
			held = append(held, h)
		}
	}
	c.held = held
}

// releaseAll lets go of every note c holds.
func (g *GM) releaseAll(c *gmChannel) {
	for _, h := range c.held {
		g.release(h)
	}
	c.held = c.held[:0]
}

func (g *GM) control(c *gmChannel, cc, v byte) {
	f := float32(v) / 127
	switch cc {
	case 0:
		c.bank = v
	case 1:
		c.mod = f
	case 6:
		if c.rpn == [2]byte{0, 0} {
			c.bendRange = float32(v)
		}
	case 7:
		c.vol = f
	case 10:
		c.pan = min(max((float32(v)-64)/63, -1), 1)
	case 11:
		c.expr = f
	case 64:
		on := v >= 64
		if c.sustain && !on {
			c.sustain = false
			g.lift(c)
		}
		c.sustain = on
	case 91:
		c.rev = f
	case 93:
		c.cho = f
	case 100:
		c.rpn[1] = v
	case 101:
		c.rpn[0] = v
	case 120:
		// All Sound Off: silence, at once.
		c.held = c.held[:0]
		for _, vc := range c.vs.vs {
			vc.on = false
		}
		for i := range c.hs.hs {
			c.hs.hs[i].on = false
		}
	case 121:
		g.resetControllers(c)
	case 123, 124, 125, 126, 127:
		// All Notes Off, and the mode changes that imply it.
		c.sustain = false
		g.releaseAll(c)
	}
}

// setProgram sets c's instrument: an instrument of General MIDI's, or
// on a drum channel a kit. A bank of 127 is XG's drums.
func (g *GM) setProgram(c *gmChannel, program int) {
	c.program = program & 127
	if c.bank == 127 {
		c.drums = true
	}
	if c.drums {
		c.kit = g.kit(c.program)
		return
	}
	c.p = g.patches[c.program]
	if c.p == nil {
		p, err := GMPatch(c.program).compile(GMNames[c.program])
		if err != nil {
			// The bank's own patches all compile; a test makes sure.
			panic(err)
		}
		g.patches[c.program] = p
		c.p = p
	}
}

// kit returns drum kit program's drums, by key.
func (g *GM) kit(program int) *[128]*drum {
	switch program {
	case 24, 25:
	default:
		program = 0
	}
	if k := g.kits[program]; k != nil {
		return k
	}
	p, err := GMKit(program).compile("kit " + strconv.Itoa(program))
	if err != nil {
		panic(err)
	}
	k := new([128]*drum)
	for key := range k {
		if d, ok := p.kit[strconv.Itoa(key)]; ok {
			k[key] = &d
		}
	}
	g.kits[program] = k
	return k
}

// sysex takes the system exclusive messages that reset the synth or
// make a channel a drum channel: General MIDI's and Roland GS's resets,
// Yamaha XG's System On, and GS's Use For Rhythm Part.
func (g *GM) sysex(b []byte) {
	is := func(prefix ...byte) bool {
		if len(b) < len(prefix) {
			return false
		}
		for i, x := range prefix {
			if x != 0xFF && b[i] != x {
				return false
			}
		}
		return true
	}
	switch {
	case is(0x7E, 0xFF, 0x09, 0x01), is(0x7E, 0xFF, 0x09, 0x03):
		g.Reset()
	case is(0x41, 0xFF, 0x42, 0x12, 0x40, 0x00, 0x7F):
		g.Reset()
	case is(0x43, 0xFF, 0x4C, 0x00, 0x00, 0x7E):
		g.Reset()
	case is(0x41, 0xFF, 0x42, 0x12, 0x40) && len(b) >= 8 && b[5]&0xF0 == 0x10 && b[6] == 0x15:
		// GS numbers its parts 1 to 9 and 0 for 10, then A to F.
		part := int(b[5] & 0x0F)
		ch := map[bool]int{true: 9, false: part - 1}[part == 0]
		if part >= 10 {
			ch = part
		}
		c := &g.ch[ch]
		g.releaseAll(c)
		c.drums = b[7] != 0
		g.setProgram(c, c.program)
	}
}

// Read fills dst with the synthesizer's sound, as interleaved stereo
// frames, and returns how many it filled. It never ends.
func (g *GM) Read(dst []float32) (int, error) {
	n := len(dst) / 2
	for at := 0; at < n; {
		m := min(maxBlock, n-at)
		g.render(dst[2*at:2*(at+m)], m)
		at += m
	}
	return n, nil
}

// Sounding reports whether a note still sounds.
func (g *GM) Sounding() bool {
	for i := range g.ch {
		for _, v := range g.ch[i].vs.vs {
			if v.on {
				return true
			}
		}
		for j := range g.ch[i].hs.hs {
			if g.ch[i].hs.hs[j].on {
				return true
			}
		}
	}
	return false
}

// render renders n frames, n up to maxBlock, into dst.
func (g *GM) render(dst []float32, n int) {
	ml, mr, rl, rr, cl, cr := g.ml[:n], g.mr[:n], g.rl[:n], g.rr[:n], g.cl[:n], g.cr[:n]
	for _, b := range [][]float32{ml, mr, rl, rr, cl, cr} {
		clear(b)
	}
	chorusOn := false
	for i := range g.ch {
		c := &g.ch[i]
		g.shift(c, n)
		tl, tr := g.tl[:n], g.tr[:n]
		clear(tl)
		clear(tr)
		var sounding bool
		if c.vs.render(tl, tr, &g.ctx) {
			sounding = true
		}
		if c.hs.render(tl, tr) {
			sounding = true
		}
		if !sounding {
			continue
		}
		gain := c.vol * c.vol * c.expr * c.expr
		pl, pr := panGains(c.pan)
		pl *= gain
		pr *= gain
		rv, cv := c.rev*0.5, c.cho*0.7
		chorusOn = chorusOn || cv > 0
		for j := range tl {
			l, r := tl[j]*pl, tr[j]*pr
			ml[j] += l
			mr[j] += r
			rl[j] += l * rv
			rr[j] += r * rv
			cl[j] += l * cv
			cr[j] += r * cv
		}
	}
	if chorusOn {
		// The chorus gives back half its input and its copies; a send
		// wants only the copies.
		dl, dr := g.dl[:n], g.dr[:n]
		copy(dl, cl)
		copy(dr, cr)
		g.cho.process(cl, cr)
		for j := range cl {
			wl, wr := cl[j]-dl[j]*0.5, cr[j]-dr[j]*0.5
			ml[j] += wl
			mr[j] += wr
			rl[j] += wl * 0.3
			rr[j] += wr * 0.3
		}
	}
	g.rev.process(rl, rr)
	for j := range ml {
		ml[j] = (ml[j] + rl[j]) * g.Gain
		mr[j] = (mr[j] + rr[j]) * g.Gain
	}
	g.lim.process(ml, mr)
	for j := range ml {
		dst[2*j] = ml[j]
		dst[2*j+1] = mr[j]
	}
}

// shift moves c's notes by its bend wheel and its modulation wheel's
// vibrato, as they stand for the next n frames.
func (g *GM) shift(c *gmChannel, n int) {
	s := c.bend * c.bendRange
	if c.mod > 0 {
		c.vibPh += 5.5 * float32(n) / rate
		c.vibPh -= float32(math.Floor(float64(c.vibPh)))
		s += sin1(c.vibPh) * c.mod * 0.5
	}
	if d := s - c.shift; d != 0 {
		for _, v := range c.vs.vs {
			if v.on {
				v.n.pitch += d
			}
		}
		c.shift = s
	}
}

// A MIDIPlayer plays a MIDI file through a [GM] synthesizer, as a
// source a mixer plays. It ends a moment after the file's last note
// dies away, unless it loops.
type MIDIPlayer struct {
	mu     sync.Mutex
	gm     *GM
	events []midi.Event
	next   int
	// frame is the frames played since the file's start.
	frame  int64
	length float64
	// tail counts the frames since the file ended and its sound died.
	tail   int
	loop   bool
	played atomic.Int64
}

// tailFrames is how long a player plays on after the last note has
// died, for the reverb's tail.
const tailFrames = 2 * rate

// NewMIDIPlayer returns a player of f, from its start.
func NewMIDIPlayer(f *midi.File) *MIDIPlayer {
	return &MIDIPlayer{gm: NewGM(), events: f.Events(), length: f.Length()}
}

// GM returns the synthesizer that plays the file.
func (p *MIDIPlayer) GM() *GM { return p.gm }

// SetLoop makes the player play the file again from its start each time
// it ends, for ever.
func (p *MIDIPlayer) SetLoop(loop bool) {
	p.mu.Lock()
	p.loop = loop
	p.mu.Unlock()
}

// Length returns how long the file plays, in seconds, to its last event.
func (p *MIDIPlayer) Length() float64 { return p.length }

// Position returns where the player is in the file, in seconds.
func (p *MIDIPlayer) Position() float64 { return float64(p.played.Load()) / rate }

// Seek moves the player to secs into the file: it silences the synth
// and plays every message before then but the notes, so each channel's
// instrument and controllers are as the file left them.
func (p *MIDIPlayer) Seek(secs float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.gm
	for i := range g.ch {
		g.control(&g.ch[i], 120, 0)
	}
	g.Reset()
	p.next, p.tail = 0, 0
	for p.next < len(p.events) && p.events[p.next].Time < secs {
		e := p.events[p.next]
		if k := e.Kind(); k != midi.NoteOn && k != midi.NoteOff {
			g.Event(e)
		}
		p.next++
	}
	p.frame = int64(max(secs, 0) * rate)
	p.played.Store(p.frame)
}

// Read fills dst with the file's sound, as interleaved stereo frames. It
// returns io.EOF once the file has ended and its sound died away.
func (p *MIDIPlayer) Read(dst []float32) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(dst) / 2
	at := 0
	for at < n {
		// Play the messages due now, then render up to the next.
		for p.next < len(p.events) && int64(p.events[p.next].Time*rate) <= p.frame {
			p.gm.Event(p.events[p.next])
			p.next++
		}
		m := n - at
		if p.next < len(p.events) {
			m = min(m, int(int64(p.events[p.next].Time*rate)-p.frame))
		} else if p.loop && p.frame > 0 {
			p.gm.Reset()
			p.next, p.frame = 0, 0
			continue
		} else {
			if !p.gm.Sounding() {
				if p.tail >= tailFrames {
					clear(dst[2*at:])
					p.played.Store(p.frame)
					return at, io.EOF
				}
				m = min(m, tailFrames-p.tail)
				p.tail += m
			}
		}
		p.gm.Read(dst[2*at : 2*(at+m)])
		at += m
		p.frame += int64(m)
	}
	p.played.Store(p.frame)
	return n, nil
}
