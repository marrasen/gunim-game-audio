package synth

import (
	"math"
	"slices"

	"github.com/marrasen/gunim/audio/band"
)

// A Beat is where a song is in its bar, for what keeps time with it, as
// a character dancing.
type Beat struct {
	// Bar is the song's bar, from 0, and Beat the beat in it, from 0 to
	// BeatsPerBar-1.
	Bar, Beat, BeatsPerBar int
	// Phase is how far through the beat it is, from 0 up to 1.
	Phase float64
	// Frame is the frame the beat started on, and Frames how long a
	// beat lasts, in frames: the next beat comes at Frame+Frames.
	Frame  int64
	Frames float64
}

// Beat returns where the song is at frame at, as heard: pass the frame
// the voice playing it is at, as for Look.
func (p *Player) Beat(at int64) Beat {
	p.wmu.Lock()
	l := p.look
	for i := p.nbars - 1; i >= max(0, p.nbars-barRing); i-- {
		if b := p.bars[i%barRing]; b.frame <= at {
			l = b.look
			break
		}
	}
	p.wmu.Unlock()
	return BeatOf(l.Bar, l.BarFrame, l.BarFrames, l.BeatsPerBar, at)
}

// BeatOf returns the beat at frame at of bar bar, which started on
// frame from and lasts frames frames, of beats beats.
func BeatOf(bar int, from int64, frames float64, beats int, at int64) Beat {
	b := Beat{Bar: bar, BeatsPerBar: max(beats, 1)}
	b.Frames = frames / float64(b.BeatsPerBar)
	if b.Frames <= 0 {
		return b
	}
	pos := max(float64(at-from)/b.Frames, 0)
	b.Beat = min(int(pos), b.BeatsPerBar-1)
	b.Phase = min(pos-float64(b.Beat), 1)
	b.Frame = from + int64(math.Round(float64(b.Beat)*b.Frames))
	return b
}

// A Hit is a drum's hit, as a dancer's claps and stamps follow it.
type Hit struct {
	// Frame is the frame it sounds on, counting as Look does.
	Frame int64
	// Track names the track that plays it.
	Track string
	// Drum names the drum as its kit does, as cp; Kind says what drum
	// it is, whatever the kit calls it: kick, snare, clap, hat, openhat,
	// rim, tom, cymbal, shaker, snap, timpani, or fx for a riser and
	// such.
	Drum, Kind string
	Vel        float32
	// Foreseen says the hit is of the bar after the one being made: it
	// sounds as foreseen unless the song changes first, as at a
	// phrase's start, where a tier changes or a part comes or goes, or
	// a sting starts.
	Foreseen bool
}

// The kinds of drums, as Hit names them.
const (
	Kick    = "kick"
	Snare   = "snare"
	Clap    = "clap"
	Hat     = "hat"
	OpenHat = "openhat"
	Rim     = "rim"
	Tom     = "tom"
	Cymbal  = "cymbal"
	Shaker  = "shaker"
	Snap    = "snap"
	Timpani = "timpani"
	FX      = "fx"
)

// drumKind returns the kind of a drum of kind k.
func drumKind(k int) string {
	switch k {
	case drKick, drSIDKick, drNESKick:
		return Kick
	case drSnare, drSIDSnare, drNESSnare:
		return Snare
	case drClap, drSIDClap, drNESClap:
		return Clap
	case drHat, drSIDHat, drNESHat:
		return Hat
	case drOHat, drSIDOHat, drNESOHat:
		return OpenHat
	case drRim, drNESMetal:
		return Rim
	case drTom, drSIDTom, drNESTom:
		return Tom
	case drCrash, drRide:
		return Cymbal
	case drShaker:
		return Shaker
	case drSnap:
		return Snap
	case drTimpani:
		return Timpani
	}
	return FX
}

// Hits appends to dst the drum hits that sound from frame from up to
// frame to, in order: those of the bar being made, and those foreseen
// in the bar after it, so a dancer has a bar's warning of each, time to
// raise its hands for a clap.
func (p *Player) Hits(dst []Hit, from, to int64) []Hit {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	start := len(dst)
	for i := max(0, p.nnotes-noteRing); i < p.nnotes; i++ {
		n := p.notes[i%noteRing]
		if n.Drum != "" && n.Frame >= from && n.Frame < to && n.Frame < p.aheadFrom {
			dst = append(dst, Hit{Frame: n.Frame, Track: n.Track, Drum: n.Drum, Kind: n.Kind, Vel: n.Vel})
		}
	}
	for _, h := range p.ahead {
		if h.Frame >= from && h.Frame < to {
			dst = append(dst, h)
		}
	}
	slices.SortStableFunc(dst[start:], func(a, b Hit) int {
		switch {
		case a.Frame < b.Frame:
			return -1
		case a.Frame > b.Frame:
			return 1
		}
		return 0
	})
	return dst
}

// foresee returns the drum hits of the bar after song bar sb, the one
// being laid out, as they will sound if the song goes on as it is: the
// tracks playing now, or at a phrase's start those its tier asks for,
// in a tiers song, and those the game turns on or off.
func (p *Player) foresee(sb int) []Hit {
	c := p.c
	if p.stopped {
		return nil
	}
	var hits []Hit
	if st := p.sting; st != nil {
		bar := p.bar + 1 - st.start
		if bar >= st.cs.s.Bars {
			// The sting ends; what comes after it is not known yet.
			return nil
		}
		for _, t := range st.tracks {
			hits = p.drumsIn(hits, t, bar)
		}
		return hits
	}
	next := sb + 1
	on := make([]bool, len(p.tracks))
	for i, t := range p.tracks {
		on[i] = t.active
	}
	if mod(next, c.phraseBars) == 0 {
		// A tiers song's parts follow its tier; a wander song's choose
		// themselves, which cannot be foreseen, so they stay as they
		// are. A part the game turns on or off follows that.
		tier := min(max(int(p.tierReq.Load()), 1), max(c.tiers, 1))
		p.mu.Lock()
		for i, t := range p.tracks {
			if !c.wander {
				on[i] = t.c.t.Tier <= tier
			}
			switch p.ctl[t.name] {
			case band.PartOn:
				on[i] = true
			case band.PartOff:
				on[i] = false
			case band.PartAuto:
			}
		}
		p.mu.Unlock()
	}
	for i, t := range p.tracks {
		if on[i] {
			hits = p.drumsIn(hits, t, next)
		}
	}
	return hits
}

// drumsIn appends to hits t's drum hits in bar, the bar after the one
// laid out, as scheduleTrack lays them out but for a human's nudges.
func (p *Player) drumsIn(hits []Hit, t *track, bar int) []Hit {
	ct := t.c
	c := p.c
	if ct.mel != nil || ct.patch.kind != kindDrums {
		return hits
	}
	bars := float64(ct.bars)
	cycle := floorDiv(bar, ct.bars)
	lo := float64(mod(bar, ct.bars)) / bars
	hi := lo + 1/bars
	for _, e := range ct.pat.query(cycle, nil) {
		act := ct.acts[e.atom]
		if act.kind != actDrum || e.at < lo-1e-9 || e.at >= hi-1e-9 {
			continue
		}
		inBar := (e.at - lo) * bars
		frame := p.barEnd + int64(inBar*c.fpb+0.5)
		if c.swing > 0 {
			s16 := inBar * 16
			if r := math.Round(s16); math.Abs(s16-r) < 1e-6 && int(r)%2 == 1 {
				frame += int64(c.swing * c.fpb / 16)
			}
		}
		vel := float32(1)
		for i := range ct.params {
			if ct.params[i].which != parVel {
				continue
			}
			if v, ok := p.paramAt(t, i, cycle, e.at); ok {
				vel = float32(v)
			}
		}
		hits = append(hits, Hit{Frame: frame, Track: t.name, Drum: act.drum.name, Kind: drumKind(act.drum.kind), Vel: vel, Foreseen: true})
	}
	return hits
}
