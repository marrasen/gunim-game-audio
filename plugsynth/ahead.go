package plugsynth

import (
	"math"

	"github.com/marrasen/gunim-game-audio/synth"
	"github.com/marrasen/gunim/audio/plug"
)

// Sounding ahead of the notes.
//
// The host tells of each note before it lands, in [plug.Block.Upcoming].
// The processor keeps the notes told of, and plays the sound on a second
// instrument, pre, ahead of each: a swell that rises into the note and
// cuts as it lands, and a quiet grace note before it, both as hard as
// the Velocity knob says. It steers them at every aheadStep frames of
// the engine's, counted from the last Reset, so they sound the same in
// any block size.

// The most notes told of the processor keeps, how many of the engine's
// frames apart it steers the sound ahead of them, how fast a swell cuts
// as its note lands and fades as its note is taken back, and how loud a
// grace note sounds, against its note.
const (
	maxAhead   = 64
	aheadStep  = 64
	swellCut   = synth.Rate / 200
	swellFade  = synth.Rate / 50
	ghostLevel = 0.35
)

// aheadNote is a note told of, still to land.
type aheadNote struct {
	note       int32
	pitch, vel float64
	// lands is the engine's frame it lands on, and turn its turn, as
	// its NoteOn will carry it.
	lands, turn int64
	// swell is pre's note swelling into it, and ghost its grace note, or
	// 0; swellPitch is the pitch the swell started at, and ghosted says
	// its grace note has sounded.
	swell, ghost int32
	swellPitch   float64
	ghosted      bool
}

// aheadNotes are the notes told of, in the order told, n of them; id
// is the last ID pre gave a note.
type aheadNotes struct {
	notes [maxAhead]aheadNote
	n     int
	id    int32
}

// find returns where note is, or -1.
func (a *aheadNotes) find(note int32) int {
	for i := range a.n {
		if a.notes[i].note == note {
			return i
		}
	}
	return -1
}

// newID returns an ID for a note pre plays.
func (a *aheadNotes) newID() int32 {
	if a.id == math.MaxInt32 {
		a.id = 0
	}
	a.id++
	return a.id
}

// restart forgets what pre plays, as a new sound takes over: the swells
// start again on it.
func (a *aheadNotes) restart() {
	for i := range a.n {
		a.notes[i].swell, a.notes[i].ghost = 0, 0
	}
}

// hear takes news of a note, at the start of the block.
func (p *processor) hear(u plug.Upcoming) {
	a := &p.ahead
	i := a.find(u.Note)
	switch u.Kind {
	case plug.Coming, plug.Moved:
		lands := int64(math.Ceil(p.engineAt(p.at + int64(u.Frame))))
		if i < 0 {
			if a.n == maxAhead {
				return
			}
			a.notes[a.n] = aheadNote{note: u.Note}
			i = a.n
			a.n++
		}
		x := &a.notes[i]
		x.pitch, x.vel, x.lands, x.turn = u.Pitch, min(max(u.Velocity, 0), 1), lands, u.Turn
		if x.swell != 0 {
			// The swell glides to where the note lands now.
			p.pre.Bend(x.swell, x.pitch-x.swellPitch)
		}
	case plug.Removed:
		if i >= 0 {
			p.endAhead(i, swellFade)
		}
	}
}

// landed ends the sound ahead of note, which lands now.
func (p *processor) landed(note int32) {
	if i := p.ahead.find(note); i >= 0 {
		p.endAhead(i, swellCut)
	}
}

// prune ends the sound ahead of the notes that should have landed
// before the engine's frame now, and forgets them.
func (p *processor) prune(now int64) {
	for i := 0; i < p.ahead.n; {
		if p.ahead.notes[i].lands+aheadStep <= now {
			p.endAhead(i, swellCut)
			continue
		}
		i++
	}
}

// endAhead ends the sound ahead of the note told of at i, its swell
// fading over fade frames, and forgets the note.
func (p *processor) endAhead(i, fade int) {
	a := &p.ahead
	x := &a.notes[i]
	if x.swell != 0 {
		p.pre.Fade(x.swell, 0, fade)
		p.pre.NoteOff(x.swell)
	}
	if x.ghost != 0 {
		p.pre.NoteOff(x.ghost)
	}
	copy(a.notes[i:a.n], a.notes[i+1:a.n])
	a.n--
}

// aheadOn reports whether the processor has sound to make ahead of the
// notes.
func (p *processor) aheadOn() bool {
	return p.ahead.n > 0 && (p.swell > 0 || p.ghost > 0) || p.pre.Sounding()
}

// renderAhead adds the sound ahead of the notes to l and r, which start
// at the engine's frame e0, steering it at each step.
func (p *processor) renderAhead(l, r []float32, e0 int64) {
	for i := 0; i < len(l); {
		at := e0 + int64(i)
		if at%aheadStep == 0 {
			p.steer(at)
		}
		k := min(len(l)-i, int(aheadStep-at%aheadStep))
		al, ar := p.al[:k], p.ar[:k]
		p.pre.Render(al, ar)
		for j := range k {
			l[i+j] += al[j]
			r[i+j] += ar[j]
		}
		i += k
	}
}

// steer starts, moves and ends the swells and grace notes at the
// engine's frame at, a step's start.
func (p *processor) steer(at int64) {
	swell := p.swell > 0 && !p.pre.Drums()
	for i := range p.ahead.n {
		x := &p.ahead.notes[i]
		switch {
		case !swell && x.swell != 0:
			p.pre.Fade(x.swell, 0, swellFade)
			p.pre.NoteOff(x.swell)
			x.swell = 0
		case swell && x.swell == 0 && at >= x.lands-p.swell && at < x.lands:
			x.swell, x.swellPitch = p.ahead.newID(), x.pitch
			p.pre.NoteOnTurn(x.swell, x.pitch, x.vel*p.aheadVel, 0, x.turn)
			p.pre.Fade(x.swell, p.rise(x, at), 0)
		}
		if x.swell != 0 {
			p.pre.Fade(x.swell, p.rise(x, min(at+aheadStep, x.lands)), aheadStep)
		}
		half := p.ghost / 2
		if p.ghost > 0 && !x.ghosted && at >= x.lands-p.ghost && at < x.lands-half {
			x.ghost, x.ghosted = p.ahead.newID(), true
			p.pre.NoteOnTurn(x.ghost, x.pitch, x.vel*ghostLevel*p.aheadVel, 0, x.turn)
		}
		if x.ghost != 0 && at >= x.lands-half {
			p.pre.NoteOff(x.ghost)
			x.ghost = 0
		}
	}
}

// rise returns the level of the swell into x at the engine's frame at:
// from 0 where the swell starts, faster and faster, to 1 where x lands.
func (p *processor) rise(x *aheadNote, at int64) float64 {
	f := float64(at-(x.lands-p.swell)) / float64(p.swell)
	f = min(max(f, 0), 1)
	return f * f
}
