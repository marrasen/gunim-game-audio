package tracker

import (
	"io"
	"math"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim-game-audio/synth"
)

// rate is the sample rate a player plays at, as package audio's.
const rate = 48000

// block is how many frames a player renders at once, at most.
const block = 256

// A Player plays a module, as a source a mixer plays: through the
// channels of a [synth.Bus], so each has its strip, and the mix the
// master's effects. It plays the song once, and the notes left ringing
// die away, unless it loops.
//
// Its methods are safe to call while a mixer reads it.
type Player struct {
	mu  sync.Mutex
	m   *Module
	bus *synth.Bus
	ch  []channel
	// Where the song is: the order, the row and the tick, and how fast:
	// ticks a row, and BPM.
	order, row, tick int
	speed, bpm       int
	global           int
	// jump and brk are the order and row a jump or a break asks for,
	// -1 for none; looped says a pattern loop asked for the row.
	jump, brk int
	looped    bool
	// delay is how many more times a pattern delay plays the row, and
	// repeating says the row is playing again.
	delay     int
	repeating bool
	// left is how many frames are left in the tick, and time how far the
	// song has played, in seconds.
	left float64
	time float64
	// pace is how fast the song plays: 1 as written.
	pace float64
	// visited are the rows played, to know the song has come round.
	visited map[int]bool
	// stopped says the song has ended, and tail counts the frames since,
	// as its last notes die away; done says they have.
	stopped, done, loop bool
	tail                int
	// out counts the frames given, and marks say where in the song each
	// run of them came from.
	out      int64
	marks    [256]mark
	nmarks   int
	played   atomic.Int64
	length   float64
	timeline *Timeline
	// rec, where set, records the rows and notes as they play.
	rec *Timeline
	buf []float32
}

type mark struct {
	out  int64
	time float64
	pace float64
}

// channel is a channel's state: the sample it plays, where, how loud
// and how high, and what its effects remember.
type channel struct {
	ins  *Instrument
	smp  *Sample
	insN int
	on   bool
	// pos is where in the sample it plays, and dir which way, back and
	// forth in a loop that goes so.
	pos float64
	dir float64
	// note0 is the note playing, 0 for C-0, its sample's relative note
	// added; period its period, and target the one a tone portamento
	// slides to.
	note0          int
	period, target float64
	finetune       int
	vol, pan       int
	keyOn          bool
	fade           int
	// The envelopes' places, in ticks.
	envVolT, envPanT int
	// The row's effect and the memories of each.
	effect, param, volCol         int
	portaUp, portaDown, portaSpd  int
	vibSpd, vibDepth, vibPos      int
	vibWave                       int
	tremSpd, tremDepth, tremPos   int
	tremWave                      int
	volSlide, fineUp, fineDown    int
	xfineUp, xfineDown, offset    int
	panSlide, globSlide, retrig   int
	tremorOn, tremorPos           int
	loopRow, loopCount            int
	delayed                       *Cell
	delayTick                     int
	autoPos, autoSweep            int
	vibDelta, tremDelta, arpShift float64
	// gain and pan, worked out each tick, and gainNow, where the gain has
	// ramped to, so a change does not click.
	gain, outPan, gainNow float32
	step                  float64
	// ghost is the note before, fading out as the new one starts.
	ghost      ghost
	tremorMute bool
}

// ghost is a note fading out under the one that took its channel.
type ghost struct {
	smp  *Sample
	pos  float64
	dir  float64
	step float64
	gain float32
}

// NewPlayer returns a player of m, from its start.
func NewPlayer(m *Module) *Player {
	p := &Player{m: m, bus: synth.NewBus(), pace: 1, buf: make([]float32, 2*block)}
	p.bus.Gain = float32(min(0.55, 1.6/math.Sqrt(float64(max(m.Channels, 1)))))
	p.reset()
	p.timeline = Scan(m)
	p.length = p.timeline.Length
	return p
}

// Timeline returns the song's rows and notes as they come, once through.
func (p *Player) Timeline() *Timeline { return p.timeline }

// reset puts the player at the song's start, silent.
func (p *Player) reset() {
	m := p.m
	p.ch = make([]channel, m.Channels)
	for i := range p.ch {
		p.ch[i].pan = 128
		if i < len(m.Pan) {
			p.ch[i].pan = m.Pan[i]
		}
		p.ch[i].dir = 1
	}
	p.order, p.row, p.tick = 0, 0, 0
	p.speed, p.bpm, p.global = max(m.Speed, 1), max(m.BPM, 32), 64
	p.jump, p.brk, p.delay, p.repeating, p.looped = -1, -1, 0, false, false
	p.left, p.time = 0, 0
	p.visited = map[int]bool{}
	p.stopped, p.done, p.tail = false, false, 0
	if len(m.Orders) == 0 {
		p.stopped = true
	}
}

// Module returns the module the player plays.
func (p *Player) Module() *Module { return p.m }

// Length returns how long the song plays, in seconds, once through.
func (p *Player) Length() float64 { return p.length }

// SetLoop makes the player play the song again each time it comes
// round, for ever, as a tracker does.
func (p *Player) SetLoop(loop bool) {
	p.mu.Lock()
	p.loop = loop
	p.mu.Unlock()
}

// SetSpeed sets how fast the song plays: 1 as written, 2 twice as fast.
func (p *Player) SetSpeed(speed float64) {
	p.mu.Lock()
	p.pace = min(max(speed, 0.1), 4)
	p.mu.Unlock()
}

// SetMute mutes channel ch, or lets it sound again.
func (p *Player) SetMute(ch int, mute bool) {
	p.mu.Lock()
	p.bus.SetMute(ch, mute)
	p.mu.Unlock()
}

// SetMix sets how the channels are mixed; see [synth.GMMix].
func (p *Player) SetMix(mix synth.GMMix) {
	p.mu.Lock()
	p.bus.SetMix(mix)
	p.mu.Unlock()
}

// TakeMeters returns how loud the mix has been since they were last
// taken.
func (p *Player) TakeMeters() synth.GMMeters {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bus.TakeMeters()
}

// Ended reports whether the song has played to its end and its last
// notes have died away.
func (p *Player) Ended() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

// Position returns how far into the song the player is, in seconds, as
// far as it has given its sound.
func (p *Player) Position() float64 { return float64(p.played.Load()) / 1e6 }

// TimeAt returns where in the song, in seconds, the out'th frame the
// player gave came from: given the frames a mixer's voice has played,
// as heard, the time heard.
func (p *Player) TimeAt(out int64) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := min(p.nmarks, len(p.marks))
	for i := 1; i <= n; i++ {
		k := p.marks[(p.nmarks-i)%len(p.marks)]
		if k.out <= out || i == n {
			return max(k.time+float64(out-k.out)*k.pace/rate, 0)
		}
	}
	return 0
}

func (p *Player) mark() {
	p.marks[p.nmarks%len(p.marks)] = mark{out: p.out, time: p.time, pace: p.pace}
	p.nmarks++
	if p.nmarks >= 2*len(p.marks) {
		p.nmarks -= len(p.marks)
	}
}

// Seek moves the player to secs into the song: it plays the song
// silently from its start to there, so every channel stands as it would.
func (p *Player) Seek(secs float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
	for !p.stopped && p.time < secs {
		p.step(true)
	}
	p.played.Store(int64(p.time * 1e6))
	p.mark()
}

// Read fills dst with the song's sound, as interleaved stereo frames. It
// returns io.EOF once the song has ended and its sound died away.
func (p *Player) Read(dst []float32) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(dst) / 2
	p.mark()
	at := 0
	for at < n {
		if p.done {
			clear(dst[2*at:])
			return at, io.EOF
		}
		m := p.render(dst[2*at:2*n], false)
		at += m
	}
	p.played.Store(int64(p.time * 1e6))
	return n, nil
}

// step plays up to the next tick, silently where dry.
func (p *Player) step(dry bool) {
	for {
		before := p.time
		p.render(nil, dry)
		if p.time != before || p.stopped {
			return
		}
	}
}

// render plays from where the player is to the end of the tick, or of
// dst, or of a block, whichever comes first, into dst, and returns how
// many frames it played. Dry, it plays silently, and as far as the tick.
func (p *Player) render(dst []float32, dry bool) int {
	if p.left < 1 {
		if !p.stopped {
			p.nextTick()
		}
		p.left += rate * 2.5 / float64(p.bpm) / p.pace
	}
	n := max(int(p.left), 1)
	if !dry {
		n = min(n, len(dst)/2, block)
	}
	p.left -= float64(n)
	p.time += float64(n) * p.pace / rate
	if dry {
		for i := range p.ch {
			p.ch[i].skip(n)
		}
		return n
	}
	b := p.bus
	b.Begin(n)
	l, r := p.buf[:n], p.buf[block:block+n]
	sounding := false
	for i := range p.ch {
		c := &p.ch[i]
		clear(l)
		if !c.mix(l) {
			continue
		}
		sounding = true
		copy(r, l)
		b.Channel(i, l, r, 1, (c.outPan-128)/128, 0.08, 0.05)
	}
	b.End(dst[:2*n])
	p.out += int64(n)
	if p.stopped {
		p.tail += n
		if !sounding && p.tail > rate/2 || p.tail > 4*rate {
			p.done = true
		}
	}
	return n
}

// nextTick plays a tick: a row's notes and effects on its first, the
// effects that move on the others, and then works out each channel's
// pitch and level.
func (p *Player) nextTick() {
	if p.tick == 0 {
		p.playRow()
	} else {
		for i := range p.ch {
			p.tickEffects(i, &p.ch[i])
		}
	}
	for i := range p.ch {
		p.output(&p.ch[i])
	}
	p.tick++
	if p.tick >= p.speed {
		p.tick = 0
		if p.delay > 0 {
			p.delay--
			p.repeating = true
			return
		}
		p.repeating = false
		p.advance()
	}
}

// advance moves to the next row, or where a jump or a break asks.
func (p *Player) advance() {
	m := p.m
	jumped := p.jump >= 0 || p.brk >= 0
	switch {
	case p.looped:
		p.row = p.brk
	case jumped:
		next := p.order + 1
		if p.jump >= 0 {
			next = p.jump
		}
		p.order = next
		p.row = max(p.brk, 0)
	default:
		p.row++
		if p.row >= p.pattern().Rows {
			p.row = 0
			p.order++
		}
	}
	wrapped := false
	if p.order >= len(m.Orders) {
		p.order = min(m.Restart, len(m.Orders)-1)
		wrapped = true
	}
	if p.row >= p.pattern().Rows {
		p.row = 0
	}
	// The song has come round where it wraps, or a jump or a break takes
	// it to a row played before; a pattern loop's rows come again by
	// design.
	key := p.order<<9 | p.row
	if !p.looped && (wrapped || jumped && p.visited[key]) {
		if !p.loop {
			p.stopped = true
		} else {
			clear(p.visited)
		}
	}
	p.jump, p.brk, p.looped = -1, -1, false
}

// pattern returns the pattern playing.
func (p *Player) pattern() *Pattern {
	o := p.m.Orders[min(p.order, len(p.m.Orders)-1)]
	return &p.m.Patterns[min(o, len(p.m.Patterns)-1)]
}

// playRow plays the row's cells: their notes, and their effects' first
// tick.
func (p *Player) playRow() {
	pat := p.pattern()
	if !p.repeating {
		p.visited[p.order<<9|p.row] = true
		if p.rec != nil {
			p.rec.Rows = append(p.rec.Rows, RowAt{Time: p.time, Order: p.order, Pattern: p.m.Orders[p.order], Row: p.row,
				Speed: p.speed, BPM: p.bpm})
		}
	}
	for i := range p.ch {
		c := &p.ch[i]
		cell := pat.Cell(p.row, i, p.m.Channels)
		if p.repeating {
			// A row played again by a pattern delay plays its effects,
			// and no notes.
			cell.Note, cell.Instrument, cell.Volume = 0, 0, 0
		}
		c.effect, c.param, c.volCol = int(cell.Effect), int(cell.Param), int(cell.Volume)
		c.vibDelta, c.tremDelta, c.arpShift = 0, 0, 0
		c.delayed = nil
		if c.effect == 0x0E && c.param>>4 == 0x0D && c.param&15 > 0 {
			cc := cell
			c.delayed, c.delayTick = &cc, c.param&15
		} else {
			p.trigger(i, c, cell)
		}
		p.firstTick(c)
	}
}

// trigger plays a cell's note, instrument and volume column.
func (p *Player) trigger(chIndex int, c *channel, cell Cell) {
	m := p.m
	porta := c.effect == 3 || c.effect == 5 || cell.Volume>>4 == 0x0F
	if cell.Instrument > 0 {
		c.insN = int(cell.Instrument)
		c.ins = nil
		if c.insN <= len(m.Instruments) {
			c.ins = &m.Instruments[c.insN-1]
		}
	}
	note := int(cell.Note)
	switch {
	case note == KeyOff:
		p.keyOff(c)
	case note > 0 && note <= 96:
		if c.ins == nil || len(c.ins.Samples) == 0 {
			if cell.Instrument > 0 {
				c.on = false
			}
			break
		}
		si := int(c.ins.Keymap[note-1])
		if si >= len(c.ins.Samples) {
			si = 0
		}
		s := &c.ins.Samples[si]
		note0 := note - 1 + s.RelNote
		if porta && c.on && c.smp != nil {
			c.target = p.notePeriod(note0, c.finetune)
			break
		}
		if c.on && c.gainNow > 0.001 && c.smp != nil {
			c.ghost = ghost{smp: c.smp, pos: c.pos, dir: c.dir, step: c.step, gain: c.gainNow}
		}
		c.smp, c.note0, c.finetune = s, note0, s.Finetune
		if c.effect == 0x0E && c.param>>4 == 5 {
			c.finetune = (c.param&15 - 8) * 16
			if c.param&15 < 8 {
				c.finetune = (c.param & 15) * 16
			}
		}
		c.period = p.notePeriod(note0, c.finetune)
		c.target = c.period
		c.pos, c.dir, c.on, c.gainNow = 0, 1, true, 0
		if c.effect == 9 {
			if c.param > 0 {
				c.offset = c.param
			}
			c.pos = float64(c.offset * 256)
			if int(c.pos) >= len(s.Data) {
				c.on = false
			}
		}
		if c.vibWave&4 == 0 {
			c.vibPos = 0
		}
		if c.tremWave&4 == 0 {
			c.tremPos = 0
		}
		c.keyOn, c.fade, c.envVolT, c.envPanT = true, 65536, 0, 0
		c.autoPos, c.autoSweep = 0, 0
		c.tremorPos = 0
		if p.rec != nil {
			p.rec.Notes = append(p.rec.Notes, NoteAt{Time: p.time, Ch: chIndex, Instrument: c.insN, Note: uint8(note),
				Sample: si, Freq: p.periodFreq(c.period)})
		}
	}
	// An instrument named sets the volume and the place its sample has.
	if cell.Instrument > 0 && c.smp != nil && note != KeyOff {
		c.vol = c.smp.Volume
		if m.Format == "XM" {
			c.pan = c.smp.Pan
			c.keyOn, c.fade, c.envVolT, c.envPanT = true, 65536, 0, 0
		}
	}
	p.volumeColumn(c, true)
}

// keyOff lets go of the note: its envelope goes on past its sustain, or
// without one the note stops.
func (p *Player) keyOff(c *channel) {
	c.keyOn = false
	if c.ins == nil || !c.ins.Vol.On {
		c.vol = 0
	}
}

// volumeColumn plays an XM's volume column: its first tick, or the ones
// after.
func (p *Player) volumeColumn(c *channel, first bool) {
	v := c.volCol
	x := v & 15
	switch v >> 4 {
	case 1, 2, 3, 4:
		if first {
			c.vol = v - 0x10
		}
	case 5:
		if first && v <= 0x50 {
			c.vol = 64
		}
	case 6:
		if !first {
			c.vol = max(c.vol-x, 0)
		}
	case 7:
		if !first {
			c.vol = min(c.vol+x, 64)
		}
	case 8:
		if first {
			c.vol = max(c.vol-x, 0)
		}
	case 9:
		if first {
			c.vol = min(c.vol+x, 64)
		}
	case 0xA:
		if first {
			c.vibSpd = x
		}
	case 0xB:
		if x > 0 {
			c.vibDepth = x
		}
		if !first {
			p.vibrato(c)
		}
	case 0xC:
		if first {
			c.pan = x * 17
		}
	case 0xD:
		if !first {
			c.pan = max(c.pan-x, 0)
		}
	case 0xE:
		if !first {
			c.pan = min(c.pan+x, 255)
		}
	case 0xF:
		if first && x > 0 {
			c.portaSpd = x * 16
		}
		if !first {
			p.tonePorta(c)
		}
	}
}

// firstTick plays the row's effect on its first tick.
func (p *Player) firstTick(c *channel) {
	m := p.m
	x, y := c.param>>4, c.param&15
	xm := m.Format == "XM"
	switch c.effect {
	case 1:
		if c.param > 0 || !xm {
			c.portaUp = c.param
		}
	case 2:
		if c.param > 0 || !xm {
			c.portaDown = c.param
		}
	case 3:
		if c.param > 0 {
			c.portaSpd = c.param
		}
	case 4:
		if x > 0 {
			c.vibSpd = x
		}
		if y > 0 {
			c.vibDepth = y
		}
	case 5, 6, 0xA:
		if c.param > 0 || !xm {
			c.volSlide = c.param
		}
	case 7:
		if x > 0 {
			c.tremSpd = x
		}
		if y > 0 {
			c.tremDepth = y
		}
	case 8:
		if xm {
			c.pan = c.param
		} else if c.param <= 0x80 {
			c.pan = min(c.param*2, 255)
		}
	case 0xB:
		p.jump = c.param
		p.brk = max(p.brk, 0)
	case 0xC:
		c.vol = min(c.param, 64)
	case 0xD:
		p.brk = min(x*10+y, 255)
	case 0xE:
		switch x {
		case 1:
			if y > 0 || !xm {
				c.fineUp = y
			}
			p.slide(c, -4*float64(c.fineUp))
		case 2:
			if y > 0 || !xm {
				c.fineDown = y
			}
			p.slide(c, 4*float64(c.fineDown))
		case 4:
			c.vibWave = y
		case 6:
			if y == 0 {
				c.loopRow = p.row
			} else if c.loopCount == 0 {
				c.loopCount = y
				p.brk, p.looped = c.loopRow, true
			} else if c.loopCount--; c.loopCount > 0 {
				p.brk, p.looped = c.loopRow, true
			}
		case 7:
			c.tremWave = y
		case 8:
			c.pan = y * 17
		case 0xA:
			c.vol = min(c.vol+y, 64)
		case 0xB:
			c.vol = max(c.vol-y, 0)
		case 0xC:
			if y == 0 {
				c.vol = 0
			}
		case 0xE:
			if !p.repeating && p.delay == 0 {
				p.delay = y
			}
		}
	case 0xF:
		switch {
		case c.param == 0:
		case c.param < 32 || m.VBlank:
			p.speed = c.param
		default:
			p.bpm = c.param
		}
	case 16: // G: global volume
		p.global = min(c.param, 64)
	case 17: // H: global volume slide
		if c.param > 0 {
			c.globSlide = c.param
		}
	case 20: // K: key off
		if c.param == 0 {
			p.keyOff(c)
		}
	case 21: // L: envelope position
		c.envVolT, c.envPanT = c.param, c.param
	case 25: // P: panning slide
		if c.param > 0 {
			c.panSlide = c.param
		}
	case 27: // R: retrigger with volume
		if c.param > 0 {
			c.retrig = c.param
		}
	case 29: // T: tremor
		if c.param > 0 {
			c.tremorOn = c.param
		}
	case 33: // X: extra fine portamento
		switch x {
		case 1:
			if y > 0 {
				c.xfineUp = y
			}
			p.slide(c, -float64(c.xfineUp))
		case 2:
			if y > 0 {
				c.xfineDown = y
			}
			p.slide(c, float64(c.xfineDown))
		}
	}
}

// tickEffects plays the row's effect on a tick after its first.
func (p *Player) tickEffects(i int, c *channel) {
	x, y := c.param>>4, c.param&15
	if c.delayed != nil && p.tick == c.delayTick {
		cell := *c.delayed
		c.delayed = nil
		p.trigger(i, c, cell)
	}
	p.volumeColumn(c, false)
	c.vibDelta, c.tremDelta, c.arpShift = 0, 0, 0
	switch c.effect {
	case 0:
		if c.param != 0 {
			switch p.tick % 3 {
			case 1:
				c.arpShift = float64(x)
			case 2:
				c.arpShift = float64(y)
			}
		}
	case 1:
		p.slide(c, -4*float64(c.portaUp))
	case 2:
		p.slide(c, 4*float64(c.portaDown))
	case 3:
		p.tonePorta(c)
	case 4:
		p.vibrato(c)
	case 5:
		p.tonePorta(c)
		p.volSlide(c)
	case 6:
		p.vibrato(c)
		p.volSlide(c)
	case 7:
		w := wave(c.tremWave, c.tremPos)
		c.tremDelta = w * float64(c.tremDepth) / 64
		c.tremPos = (c.tremPos + c.tremSpd) & 63
	case 0xA:
		p.volSlide(c)
	case 0xE:
		switch x {
		case 9:
			if y > 0 && p.tick%y == 0 {
				c.pos, c.dir, c.on = 0, 1, c.smp != nil
			}
		case 0xC:
			if p.tick == y {
				c.vol = 0
			}
		}
	case 17:
		gx, gy := c.globSlide>>4, c.globSlide&15
		if gx > 0 {
			p.global = min(p.global+gx, 64)
		} else {
			p.global = max(p.global-gy, 0)
		}
	case 20:
		if p.tick == c.param {
			p.keyOff(c)
		}
	case 25:
		px, py := c.panSlide>>4, c.panSlide&15
		if px > 0 {
			c.pan = min(c.pan+px, 255)
		} else {
			c.pan = max(c.pan-py, 0)
		}
	case 27:
		rx, ry := c.retrig>>4, c.retrig&15
		if ry > 0 && p.tick%ry == 0 {
			c.pos, c.dir, c.on = 0, 1, c.smp != nil
			c.vol = retrigVolume(c.vol, rx)
		}
	case 29:
		on, off := c.tremorOn>>4+1, c.tremorOn&15+1
		c.tremorMute = c.tremorPos%(on+off) >= on
		c.tremorPos++
	}
}

// retrigVolume changes a volume as a multi-retrigger's x says.
func retrigVolume(v, x int) int {
	switch x {
	case 1, 2, 3, 4, 5:
		v -= 1 << (x - 1)
	case 6:
		v = v * 2 / 3
	case 7:
		v /= 2
	case 9, 10, 11, 12, 13:
		v += 1 << (x - 9)
	case 14:
		v = v * 3 / 2
	case 15:
		v *= 2
	}
	return min(max(v, 0), 64)
}

func (p *Player) volSlide(c *channel) {
	x, y := c.volSlide>>4, c.volSlide&15
	if x > 0 {
		c.vol = min(c.vol+x, 64)
	} else {
		c.vol = max(c.vol-y, 0)
	}
}

// slide moves the channel's period by d, within the module's bounds.
func (p *Player) slide(c *channel, d float64) {
	c.period = p.clampPeriod(c.period + d)
}

func (p *Player) clampPeriod(per float64) float64 {
	switch {
	case p.m.Linear:
		return min(max(per, 1), 7680*2)
	case p.m.Format == "MOD":
		return min(max(per, 113*4), 856*4)
	}
	return min(max(per, 1), 32000)
}

func (p *Player) tonePorta(c *channel) {
	d := float64(c.portaSpd * 4)
	if c.period < c.target {
		c.period = min(c.period+d, c.target)
	} else if c.period > c.target {
		c.period = max(c.period-d, c.target)
	}
}

func (p *Player) vibrato(c *channel) {
	w := wave(c.vibWave, c.vibPos)
	scale := 4.0
	if p.m.Linear {
		scale = 2.5
	}
	c.vibDelta = w * float64(c.vibDepth) / 128 * scale
	c.vibPos = (c.vibPos + c.vibSpd) & 63
}

// wave returns a vibrato's or a tremolo's wave at pos, 0 to 63, from
// -255 to 255: a sine, a falling ramp, a square, or random.
func wave(kind, pos int) float64 {
	switch kind & 3 {
	case 1:
		return 255 - float64(pos)*8.1
	case 2:
		if pos < 32 {
			return 255
		}
		return -255
	case 3:
		h := uint32(pos*2654435761) >> 24
		return float64(h)*2 - 255
	}
	return 255 * math.Sin(float64(pos)*2*math.Pi/64)
}

// notePeriod returns the period of note0, 0 for C-0, finetuned.
func (p *Player) notePeriod(note0, ft int) float64 {
	if p.m.Linear {
		return 7680 - float64(note0)*64 - float64(ft)/2
	}
	return 1712 * math.Pow(2, -(float64(note0)-48)/12-float64(ft)/1536)
}

// periodFreq returns the rate a period plays its sample at.
func (p *Player) periodFreq(per float64) float64 {
	if p.m.Linear {
		return 8363 * math.Pow(2, (4608-per)/768)
	}
	return p.m.clock / max(per, 1)
}

// output works out the channel's pitch and level for the tick: its
// period with the vibrato and the arpeggio, its volume with the
// tremolo, the envelopes, the fadeout and the global volume.
func (p *Player) output(c *channel) {
	if !c.on || c.smp == nil {
		c.gain = 0
		return
	}
	per := c.period + c.vibDelta
	// The instrument's own vibrato.
	if in := c.ins; in != nil && in.VibDepth > 0 {
		depth := float64(in.VibDepth)
		if in.VibSweep > 0 && c.autoSweep < in.VibSweep {
			depth *= float64(c.autoSweep) / float64(in.VibSweep)
			c.autoSweep++
		}
		w := wave([]int{0, 2, 1, 1}[in.VibType&3], (c.autoPos/4)&63)
		per += w * depth / 256 * 2
		c.autoPos += in.VibRate
	}
	freq := p.periodFreq(p.clampPeriod(per))
	if c.arpShift != 0 {
		freq *= math.Pow(2, c.arpShift/12)
	}
	c.step = freq / rate
	vol := min(max(float64(c.vol)+c.tremDelta, 0), 64) / 64
	pan := float64(c.pan)
	if in := c.ins; in != nil && p.m.Format == "XM" {
		if in.Vol.On {
			vol *= envAt(&in.Vol, c.envVolT) / 64
			c.envVolT = envNext(&in.Vol, c.envVolT, c.keyOn)
		}
		if in.Pan.On {
			e := envAt(&in.Pan, c.envPanT) - 32
			pan += e * (128 - math.Abs(pan-128)) / 32
			c.envPanT = envNext(&in.Pan, c.envPanT, c.keyOn)
		}
		if !c.keyOn {
			vol *= float64(c.fade) / 65536
			c.fade = max(c.fade-in.Fadeout*2, 0)
			if c.fade == 0 {
				c.on = false
			}
		}
	}
	if c.tremorMute && c.effect == 29 {
		vol = 0
	}
	c.gain = float32(vol * float64(p.global) / 64)
	c.outPan = float32(min(max(pan, 0), 255))
}

// envAt returns envelope e's value at tick t, between its points.
func envAt(e *Envelope, t int) float64 {
	pts := e.Points
	if len(pts) == 0 {
		return 64
	}
	if t <= pts[0][0] {
		return float64(pts[0][1])
	}
	for i := 1; i < len(pts); i++ {
		if t <= pts[i][0] {
			a, b := pts[i-1], pts[i]
			if b[0] == a[0] {
				return float64(b[1])
			}
			f := float64(t-a[0]) / float64(b[0]-a[0])
			return float64(a[1]) + f*float64(b[1]-a[1])
		}
	}
	return float64(pts[len(pts)-1][1])
}

// envNext returns the envelope's tick after t: held at its sustain
// while the key is, and round its loop.
func envNext(e *Envelope, t int, keyOn bool) int {
	pts := e.Points
	if len(pts) == 0 {
		return t
	}
	if e.Sustain && keyOn && e.SustainAt < len(pts) && t == pts[e.SustainAt][0] {
		return t
	}
	t++
	if e.Loop && e.LoopEnd < len(pts) && e.LoopStart < len(pts) && t >= pts[e.LoopEnd][0] {
		t = pts[e.LoopStart][0]
	}
	return t
}

// mix adds the channel's sound to out, its gain gliding from where it
// was to where the tick puts it, and its note before fading under it.
// It reports whether the channel sounded.
func (c *channel) mix(out []float32) bool {
	sounded := false
	if g := &c.ghost; g.smp != nil && g.gain > 0 {
		// The note before fades out over 2 ms.
		n := min(len(out), 96)
		for i := range n {
			s, ok := readSample(g.smp, &g.pos, &g.dir, g.step)
			if !ok {
				break
			}
			out[i] += s * g.gain * (1 - float32(i+1)/float32(n))
		}
		g.smp = nil
		sounded = true
	}
	if !c.on || c.smp == nil {
		c.gainNow = 0
		return sounded
	}
	from, to := c.gainNow, c.gain
	ramp := min(len(out), 64)
	for i := range out {
		g := to
		if i < ramp {
			g = from + (to-from)*float32(i+1)/float32(ramp)
		}
		s, ok := readSample(c.smp, &c.pos, &c.dir, c.step)
		if !ok {
			c.on = false
			break
		}
		out[i] += s * g
	}
	c.gainNow = to
	return true
}

// skip moves the channel on n frames, silently.
func (c *channel) skip(n int) {
	c.ghost.smp = nil
	if !c.on || c.smp == nil {
		return
	}
	c.gainNow = c.gain
	s := c.smp
	c.pos += c.step * float64(n) * c.dir
	ls, ll := float64(s.LoopStart), float64(s.LoopLen)
	end := ls + ll
	switch {
	case s.Loop == 1 && ll > 0 && c.pos >= end:
		c.pos = ls + math.Mod(c.pos-ls, ll)
	case s.Loop == 2 && ll > 0 && (c.pos >= end || c.pos < ls && c.dir < 0):
		d := math.Mod(c.pos-ls, 2*ll)
		if d < 0 {
			d += 2 * ll
		}
		if d < ll {
			c.pos, c.dir = ls+d, 1
		} else {
			c.pos, c.dir = end-(d-ll), -1
		}
	case s.Loop == 0 && c.pos >= float64(len(s.Data)):
		c.on = false
	}
}

// readSample reads sample s at *pos, between its frames, and moves *pos
// on by step, round its loop; it reports false once the sample has
// ended.
func readSample(s *Sample, pos, dir *float64, step float64) (float32, bool) {
	d := s.Data
	ip := int(*pos)
	if ip < 0 || ip >= len(d) {
		return 0, false
	}
	end := len(d)
	if s.Loop != 0 {
		end = s.LoopStart + s.LoopLen
	}
	next := ip + 1
	if next >= end {
		switch s.Loop {
		case 1:
			next = s.LoopStart
		default:
			next = ip
		}
	}
	f := float32(*pos - float64(ip))
	v := d[ip] + (d[min(next, len(d)-1)]-d[ip])*f
	*pos += step * *dir
	ls, le := float64(s.LoopStart), float64(end)
	switch s.Loop {
	case 1:
		for *pos >= le && s.LoopLen > 0 {
			*pos -= float64(s.LoopLen)
		}
	case 2:
		if *dir > 0 && *pos >= le {
			*pos, *dir = 2*le-*pos-1, -1
		} else if *dir < 0 && *pos < ls {
			*pos, *dir = 2*ls-*pos, 1
		}
		*pos = min(max(*pos, ls), le-1)
	}
	return v, true
}

// A Timeline is a song's rows and notes as they come, played through
// once, silently: for a tool that shows the song as it plays.
type Timeline struct {
	Rows   []RowAt
	Notes  []NoteAt
	Length float64
}

// RowAt says a row plays from Time seconds: Row of the pattern Pattern,
// at Order, at Speed ticks a row and BPM.
type RowAt struct {
	Time                float64
	Order, Pattern, Row int
	Speed, BPM          int
}

// NoteAt says channel Ch starts a note at Time seconds: Note, of
// Instrument's Sample, at Freq hertz.
type NoteAt struct {
	Time       float64
	Ch         int
	Instrument int
	Note       uint8
	Sample     int
	Freq       float64
}

// Scan plays m through once, silently, and returns its rows and notes as
// they come.
func Scan(m *Module) *Timeline {
	p := &Player{m: m, bus: nil, pace: 1}
	p.reset()
	p.rec = &Timeline{}
	for !p.stopped && p.time < 60*60 {
		p.step(true)
	}
	p.rec.Length = p.time
	return p.rec
}
