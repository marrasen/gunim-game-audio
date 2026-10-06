package calls

import (
	"math/rand/v2"
	"sync"

	"github.com/marrasen/gunim/audio"
)

// ahead is how many takes of each call a player keeps made, ready to
// play.
const ahead = 3

// A Player plays the companions' calls through a mixer, a new take each
// time, so a call tapped again and again never sounds the same twice.
// It makes takes ahead, in the background, so each plays at once.
type Player struct {
	mix *audio.Mixer
	lib *Library

	mu    sync.Mutex
	ready map[string][]*audio.Clip
	busy  map[string]bool
	// Volume scales every call, 1 as made.
	Volume float32
}

// NewPlayer returns a player of lib's calls through mix.
func NewPlayer(mix *audio.Mixer, lib *Library) *Player {
	return &Player{mix: mix, lib: lib, ready: map[string][]*audio.Clip{}, busy: map[string]bool{}, Volume: 1}
}

// Warm makes takes of every call of the companions ids ahead, in the
// background, so even each first call plays at once. Without ids it
// warms every companion's.
func (p *Player) Warm(ids ...string) {
	if len(ids) == 0 {
		for _, c := range p.lib.Companions {
			ids = append(ids, c.ID)
		}
	}
	for _, id := range ids {
		c := p.lib.Companion(id)
		if c == nil {
			continue
		}
		for kind := range c.Calls {
			p.refill(id, kind)
		}
	}
}

// Play plays companion id's call of kind, a take of its own, and
// returns its voice.
func (p *Player) Play(id, kind string, o audio.Options) (*audio.Voice, error) {
	key := id + "/" + kind
	p.mu.Lock()
	var clip *audio.Clip
	if q := p.ready[key]; len(q) > 0 {
		clip, p.ready[key] = q[0], q[1:]
	}
	p.mu.Unlock()
	if clip == nil {
		t, err := p.lib.Take(id, kind, newSeed())
		if err != nil {
			return nil, err
		}
		clip = t.Clip()
	}
	p.refill(id, kind)
	return p.play(clip, o), nil
}

// PlayTake plays take seed of companion id's call of kind: seed 0 is
// the call as set, the same each time.
func (p *Player) PlayTake(id, kind string, seed uint64, o audio.Options) (*audio.Voice, error) {
	t, err := p.lib.Take(id, kind, seed)
	if err != nil {
		return nil, err
	}
	return p.play(t.Clip(), o), nil
}

func (p *Player) play(c *audio.Clip, o audio.Options) *audio.Voice {
	if o.Volume == 0 {
		o.Volume = 1
	}
	o.Volume *= p.Volume
	return p.mix.Play(c.Source(), o)
}

// refill makes takes of the call in the background until ahead are
// ready.
func (p *Player) refill(id, kind string) {
	key := id + "/" + kind
	p.mu.Lock()
	if p.busy[key] || len(p.ready[key]) >= ahead {
		p.mu.Unlock()
		return
	}
	p.busy[key] = true
	p.mu.Unlock()
	go func() {
		for {
			t, err := p.lib.Take(id, kind, newSeed())
			p.mu.Lock()
			if err == nil {
				p.ready[key] = append(p.ready[key], t.Clip())
			}
			if err != nil || len(p.ready[key]) >= ahead {
				p.busy[key] = false
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
		}
	}()
}

// newSeed returns a seed for a new take, never 0, the call as set.
func newSeed() uint64 { return rand.Uint64() | 1 }
