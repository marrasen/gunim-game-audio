package calls

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/marrasen/gunim/audio"
)

// rate is the rate calls are made at, the mixer's.
const rate = float64(audio.SampleRate)

// The calls each companion makes.
const (
	// Hello plays when a child taps or picks the companion.
	Hello = "hello"
	// Cheer plays for a level done, a chest opened or a new level of
	// the companion.
	Cheer = "cheer"
	// Oops plays for a wrong answer: kind and light.
	Oops = "oops"
)

// Kinds are the calls each companion makes, in order.
var Kinds = []string{Hello, Cheer, Oops}

// A Library is the companions and how their calls are finished.
type Library struct {
	Master Master
	// Companions are the companions in the order they are shown.
	Companions []*Companion `json:"-"`
	// Order is the companions' IDs, in order, as library.json lists
	// them.
	Order []string
}

// Master is how every call is finished, so they sit together: cut
// below a pitch, as a phone's speaker plays nothing low, and brought to
// one loudness under a ceiling.
type Master struct {
	// HighPass cuts below it, in hertz.
	HighPass float64
	// Loudness is how loud each call is at its loudest, in LUFS over
	// 400 ms, as BS.1770's momentary loudness.
	Loudness float64
	// Ceiling is the highest a call's true peak goes, in dBTP.
	Ceiling float64
}

// DefaultMaster is how calls are finished where the library says
// nothing.
var DefaultMaster = Master{HighPass: 250, Loudness: -14, Ceiling: -1}

// A Companion is a character of the game and its calls.
type Companion struct {
	ID        string `json:"-"`
	Name      string
	Style     string
	Character string
	// Calls are its calls by kind: Hello, Cheer and Oops.
	Calls map[string]*Call
}

// A Call is a sound a companion makes: layers of models of sound, each
// at a time, in a room.
type Call struct {
	Layers []*Layer
	// Vary is how far each take strays from the call as set, 0 not at
	// all and 1 as far as its models let it.
	Vary float64
	// Room is how much of a small room is heard round the call, 0 to 1.
	Room float64
	// Notes is what the listener asks to change, for the call's next
	// round.
	Notes string `json:",omitempty"`
}

// A Layer is a model of a sound, at a time in a call.
type Layer struct {
	Model string
	// At is when it starts, in seconds; Gain its level, in decibels.
	At   float64 `json:",omitempty"`
	Gain float64 `json:",omitempty"`
	// Params are its model's numbers by name; those left out are their
	// defaults.
	Params map[string]float64
}

// Get returns the value of the parameter name, or its default.
func (l *Layer) Get(name string) float64 {
	if v, ok := l.Params[name]; ok {
		return v
	}
	if m, ok := models[l.Model]; ok {
		for _, p := range m.Params {
			if p.Name == name {
				return p.Def
			}
		}
	}
	return 0
}

// Load reads a library from fsys: its library.json, and a file for each
// companion it lists, named for its ID.
func Load(fsys fs.FS) (*Library, error) {
	l := &Library{Master: DefaultMaster}
	b, err := fs.ReadFile(fsys, "library.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, l); err != nil {
		return nil, fmt.Errorf("calls: library.json: %w", err)
	}
	for _, id := range l.Order {
		b, err := fs.ReadFile(fsys, id+".json")
		if errors.Is(err, fs.ErrNotExist) {
			l.Companions = append(l.Companions, &Companion{ID: id, Name: id, Calls: map[string]*Call{}})
			continue
		}
		if err != nil {
			return nil, err
		}
		c := &Companion{ID: id}
		if err := json.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("calls: %s.json: %w", id, err)
		}
		if c.Calls == nil {
			c.Calls = map[string]*Call{}
		}
		for kind, call := range c.Calls {
			if !slices.Contains(Kinds, kind) {
				return nil, fmt.Errorf("calls: %s makes a call %q, not hello, cheer or oops", id, kind)
			}
			for _, ly := range call.Layers {
				if _, ok := models[ly.Model]; !ok {
					return nil, fmt.Errorf("calls: %s's %s has a layer of model %q, which is not one of %v", id, kind, ly.Model, ModelNames())
				}
			}
		}
		l.Companions = append(l.Companions, c)
	}
	return l, nil
}

// LoadDir reads a library from the folder dir.
func LoadDir(dir string) (*Library, error) { return Load(os.DirFS(dir)) }

// Companion returns the companion of ID id, or nil.
func (l *Library) Companion(id string) *Companion {
	for _, c := range l.Companions {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Save writes the library's settings and companion c to the folder dir,
// each parameter written out, so the file shows all a call's numbers.
func (l *Library) Save(dir string, c *Companion) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, call := range c.Calls {
		for _, ly := range call.Layers {
			if ly.Params == nil {
				ly.Params = map[string]float64{}
			}
			for _, p := range models[ly.Model].Params {
				if _, ok := ly.Params[p.Name]; !ok {
					ly.Params[p.Name] = p.Def
				}
			}
		}
	}
	if err := writeJSON(filepath.Join(dir, "library.json"), l); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, c.ID+".json"), c)
}

func writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(b, '\n'), 0o644)
}

// A Take is a call as made once: its samples, mono at the mixer's rate,
// and how it measures.
type Take struct {
	Samples []float32
	Stats   Stats
	Seed    uint64
}

// Clip returns the take as a clip, the same in both speakers.
func (t *Take) Clip() *audio.Clip {
	st := make([]float32, 2*len(t.Samples))
	for i, v := range t.Samples {
		st[2*i], st[2*i+1] = v, v
	}
	return audio.NewClip(st)
}

// Take makes companion id's call of kind, its take seed: seed 0 is the
// call as set, and every other seed strays from it by the call's Vary.
func (l *Library) Take(id, kind string, seed uint64) (*Take, error) {
	c := l.Companion(id)
	if c == nil {
		return nil, fmt.Errorf("calls: no companion %q", id)
	}
	call, ok := c.Calls[kind]
	if !ok {
		return nil, fmt.Errorf("calls: %s makes no %s call yet", id, kind)
	}
	return l.Make(call, seed), nil
}

// Make makes a take of call, its take seed. A call that does not vary
// makes every take as set.
func (l *Library) Make(call *Call, seed uint64) *Take {
	if call.Vary == 0 {
		seed = 0
	}
	r := newRNG(seed)
	var mix []float64
	for i, ly := range call.Layers {
		m, ok := models[ly.Model]
		if !ok {
			continue
		}
		p := params{}
		for _, spec := range m.Params {
			p[spec.Name] = spec.stray(ly.Get(spec.Name), call.Vary, seed, r)
		}
		out := m.render(p, newRNG(seed^0x51ed270b2c8f3a61+uint64(i)*0x7f4a7c15))
		g := dB(ly.Gain)
		at := int(max(ly.At, 0) * rate)
		if need := at + len(out); need > len(mix) {
			mix = append(mix, make([]float64, need-len(mix))...)
		}
		for i, v := range out {
			mix[at+i] += v * g
		}
	}
	samples, st := l.master(mix, call.Room)
	return &Take{Samples: samples, Stats: st, Seed: seed}
}

// params are a model's numbers by name.
type params map[string]float64

// A Param is a number a model takes, as its pitch.
type Param struct {
	Name, Label, Unit string
	// Lo and Hi bound it, and Def is where it starts; Log turns it by
	// ratios, as a pitch; Step rounds it, as a count.
	Lo, Hi, Def float64
	Log         bool
	Step        float64
	// Vary is how far a take strays from the value, at a call's Vary 1:
	// a deviation of that fraction of the range, or for a Log
	// parameter of the value.
	Vary float64
	// About says what it does.
	About string
}

// stray returns v as take seed has it, strayed by vary, within the
// parameter's range: seed 0 is v itself.
func (p Param) stray(v, vary float64, seed uint64, r *rng) float64 {
	d := r.norm()
	if seed == 0 || vary == 0 || p.Vary == 0 {
		return p.clamp(v)
	}
	if p.Log {
		v *= math.Exp(d * p.Vary * vary)
	} else {
		v += d * p.Vary * vary * (p.Hi - p.Lo)
	}
	if p.Step > 0 {
		v = math.Round(v/p.Step) * p.Step
	}
	return p.clamp(v)
}

func (p Param) clamp(v float64) float64 { return min(max(v, p.Lo), p.Hi) }

// A Model makes a kind of sound from its numbers, as a hoot or a
// bubble.
type Model struct {
	Name, About string
	Params      []Param
	render      func(p params, r *rng) []float64
}

var models = map[string]*Model{}

func register(m *Model) { models[m.Name] = m }

// Models returns the models by name.
func Models() map[string]*Model { return models }

// ModelNames returns the models' names, sorted.
func ModelNames() []string {
	names := make([]string, 0, len(models))
	for n := range models {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// FileName returns the name of the file of companion id's call of
// kind, as voices/ holds it.
func FileName(id, kind, ext string) string { return path.Clean(id + "-" + kind + "." + ext) }
