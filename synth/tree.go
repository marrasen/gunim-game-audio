package synth

import (
	"strconv"
	"strings"
)

// A Step is a node of a pattern, as a tool shows and edits it: an atom,
// a rest, or a node over others. ParsePattern reads one, and String
// writes it back as mini-notation.
type Step struct {
	Kind StepKind
	// Atom is an atom's word, as bd or c2.
	Atom string
	// Kids are what a node holds: a sequence's steps, a stack's layers,
	// an alternation's turns, or the one step a modifier changes.
	Kids []*Step
	// Weight is the step's share of its sequence, 1 by default.
	Weight float64
	// N is how many times faster or slower a Fast or Slow plays.
	N int
	// Hits, Steps and Rotate are a Euclid's rhythm, as (3,8,2).
	Hits, Steps, Rotate int
	// Chance is how often a Degrade drops an event, from 0 to 1.
	Chance float64
}

// StepKind is the kind of a Step.
type StepKind int

// The kinds of Step.
const (
	// StepAtom is a word: a note, a drum or a value.
	StepAtom StepKind = iota
	// StepRest is a rest, ~.
	StepRest
	// StepSeq is steps sharing their span by weight, as [a b c].
	StepSeq
	// StepStack is layers at once, as [a, b].
	StepStack
	// StepAlt is turns, one a cycle, as <a b c>.
	StepAlt
	// StepFast plays its step N times in its span, as a*2.
	StepFast
	// StepSlow plays its step over N spans, as a/2.
	StepSlow
	// StepEuclid spreads Hits of its step over Steps, as a(3,8).
	StepEuclid
	// StepDegrade drops its events by Chance, as a?.
	StepDegrade
)

// ParsePattern reads a pattern in mini-notation into its steps.
func ParsePattern(src string) (*Step, error) {
	p, err := readPattern(src)
	if err != nil {
		return nil, err
	}
	return p.step(p.root), nil
}

func (p *pattern) step(n *pnode) *Step {
	s := &Step{Weight: n.weight}
	if s.Weight == 0 {
		s.Weight = 1
	}
	switch n.kind {
	case pAtom:
		s.Kind, s.Atom = StepAtom, p.atoms[n.atom]
	case pRest:
		s.Kind = StepRest
	case pSeq:
		s.Kind = StepSeq
	case pStack:
		s.Kind = StepStack
	case pAlt:
		s.Kind = StepAlt
	case pFast:
		s.Kind, s.N = StepFast, n.n
	case pSlow:
		s.Kind, s.N = StepSlow, n.n
	case pEuclid:
		s.Kind, s.Hits, s.Steps, s.Rotate = StepEuclid, n.euclid[0], n.euclid[1], n.euclid[2]
	case pDegrade:
		s.Kind, s.Chance = StepDegrade, n.chance
	}
	for _, k := range n.kids {
		s.Kids = append(s.Kids, p.step(k))
	}
	return s
}

// String writes the step as mini-notation, as ParsePattern reads it.
func (s *Step) String() string {
	switch s.Kind {
	case StepSeq:
		return s.join(" ")
	case StepStack:
		return s.join(", ")
	default:
	}
	return s.write()
}

// join writes the step's kids, split by sep.
func (s *Step) join(sep string) string {
	parts := make([]string, len(s.Kids))
	for i, k := range s.Kids {
		parts[i] = k.write() + weight(k.Weight)
	}
	return strings.Join(parts, sep)
}

func weight(w float64) string {
	if w == 1 || w == 0 {
		return ""
	}
	return "@" + strconv.FormatFloat(w, 'f', -1, 64)
}

// write writes the step as one term, bracketed where it holds several.
func (s *Step) write() string {
	kid := func() string {
		if len(s.Kids) == 0 {
			return "~"
		}
		return s.Kids[0].write()
	}
	switch s.Kind {
	case StepAtom:
		return s.Atom
	case StepRest:
		return "~"
	case StepSeq:
		if len(s.Kids) == 1 {
			return s.Kids[0].write()
		}
		return "[" + s.join(" ") + "]"
	case StepStack:
		return "[" + s.join(", ") + "]"
	case StepAlt:
		return "<" + s.join(" ") + ">"
	case StepFast:
		return kid() + "*" + strconv.Itoa(max(s.N, 1))
	case StepSlow:
		return kid() + "/" + strconv.Itoa(max(s.N, 1))
	case StepEuclid:
		e := "(" + strconv.Itoa(s.Hits) + "," + strconv.Itoa(max(s.Steps, 1))
		if s.Rotate != 0 {
			e += "," + strconv.Itoa(s.Rotate)
		}
		return kid() + e + ")"
	case StepDegrade:
		if s.Chance == 0.5 || s.Chance == 0 {
			return kid() + "?"
		}
		return kid() + "?" + strconv.FormatFloat(s.Chance, 'f', -1, 64)
	}
	return "~"
}

// Event is an event of a pattern's cycle, as a tool shows it: where it
// starts and how long it lasts, from 0 to 1 of the cycle, and its atom.
type Event struct {
	At, Dur float64
	Atom    string
}

// PatternEvents returns the events of cycle of the pattern src, in the
// order they start.
func PatternEvents(src string, cycle int) ([]Event, error) {
	p, err := readPattern(src)
	if err != nil {
		return nil, err
	}
	evs := p.query(cycle, nil)
	out := make([]Event, len(evs))
	for i, e := range evs {
		out[i] = Event{At: e.at, Dur: e.dur, Atom: p.atoms[e.atom]}
	}
	return out, nil
}

// PatternCycles returns how many cycles a pattern takes to come round to
// the same again, as its alternations and slowings make it: 1 for a
// pattern of none, and at most 64.
func PatternCycles(src string) int {
	p, err := readPattern(src)
	if err != nil {
		return 1
	}
	return min(cycles(p.root), 64)
}

func cycles(n *pnode) int {
	c := 1
	for _, k := range n.kids {
		c = lcm(c, cycles(k))
	}
	switch n.kind {
	case pAlt:
		c *= len(n.kids)
	case pSlow:
		c *= n.n
	case pFast:
		c = max(1, c/gcd(c, n.n))
	default:
	}
	return min(c, 64)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return max(a, 1)
}

func lcm(a, b int) int { return a / gcd(a, b) * b }

// DrumNames are the drums a drums patch plays by name, in the order a
// tool lists them.
var DrumNames = []string{"bd", "sn", "cp", "rim", "hh", "oh", "sh", "snap", "lt", "mt", "ht", "cr", "rd", "tim", "boom", "riser", "down",
	"syn1", "syn2", "syn3", "cb", "bd9", "mtl", "sbd", "ssn", "scp", "shh", "soh", "stom", "szap", "nbd", "nsn", "ncp", "nhh", "noh", "ntom", "nclk"}

// DrumTypes are the types of drum, in the order a tool lists them.
var DrumTypes = []string{"kick", "snare", "clap", "hat", "ohat", "rim", "tom", "crash", "ride", "shaker", "snap", "timpani", "boom", "riser", "down",
	"syntom", "cowbell", "kick909", "metal", "sidkick", "sidsnare", "sidclap", "sidhat", "sidohat", "sidtom", "sidzap",
	"neskick", "nessnare", "nesclap", "neshat", "nesohat", "nestom", "nesmetal"}

// DrumType returns the type of the drum named name in p, as the kit
// gives it or its name does.
func (p *Patch) DrumType(name string) string {
	if d, ok := p.Kit[name]; ok && d.Type != "" {
		return d.Type
	}
	return drumNames[name]
}
