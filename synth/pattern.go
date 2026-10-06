package synth

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A pattern is a cycle of events written in mini-notation, as
// TidalCycles writes them. Its steps share the cycle evenly:
//
//	bd ~ sn ~        four steps, two of them rests
//	[c0 c1]*2 c2     a step of two, played twice, and one more
//	<0 2 4>          one of the three each cycle, in turn
//	bd(3,8)          three hits spread over eight steps, Euclid's way
//	hh*8?            eight hats, each left out half the time
//	0 _ 2 4@2        a step held for two, and a step weighing two
//	[0, 2, 4]        three at once
//	0 2 . 4 5 7      groups: [0 2] [4 5 7]
//
// Each word is an atom, which a track reads as it needs to: a note, a
// drum or a value.
type pattern struct {
	src   string
	root  *pnode
	atoms []string
}

// The kinds of a pattern's node.
type pkind uint8

const (
	pAtom pkind = iota
	pRest
	pSeq
	pStack
	pAlt
	pFast
	pSlow
	pEuclid
	pDegrade
)

// pnode is a node of a pattern: an atom, a rest, or a node over others.
type pnode struct {
	kind pkind
	// atom is the atom's index, for pAtom.
	atom int
	kids []*pnode
	// weight is the node's share of its sequence: 1, or more for @ or a
	// following _.
	weight float64
	// n is how many times faster or slower, for pFast and pSlow.
	n int
	// hits are a pEuclid's steps, true where its node plays.
	hits []bool
	// chance is how often a pDegrade drops an event, and salt seeds it.
	chance float64
	salt   uint32
}

// pev is an event a query finds: where it starts in the cycle, and how
// long it lasts, both from 0 to 1, and its atom.
type pev struct {
	at, dur float64
	atom    int
}

// parsePattern reads src.
func parsePattern(src string) (*pattern, error) {
	p := &parser{src: src, pat: &pattern{src: src}}
	root, err := p.list(0)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.src) {
		return nil, p.errorf("unexpected %q", p.src[p.pos])
	}
	p.pat.root = root
	return p.pat, nil
}

// mustPattern is parsePattern for patterns known to be good.
func mustPattern(src string) *pattern {
	p, err := parsePattern(src)
	if err != nil {
		panic(err)
	}
	return p
}

type parser struct {
	src  string
	pos  int
	pat  *pattern
	salt uint32
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("pattern %q, at %d: %s", p.src, p.pos+1, fmt.Sprintf(format, args...))
}

func (p *parser) skip() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '|') {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skip()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

// list reads sequences split by commas, up to close or the end, as a
// stack where there are several.
func (p *parser) list(closer byte) (*pnode, error) {
	var seqs []*pnode
	for {
		s, err := p.seq(closer)
		if err != nil {
			return nil, err
		}
		seqs = append(seqs, s)
		if p.peek() != ',' {
			break
		}
		p.pos++
	}
	if len(seqs) == 1 {
		return seqs[0], nil
	}
	return &pnode{kind: pStack, kids: seqs, weight: 1}, nil
}

// seq reads steps up to a comma, close or the end. Dots split them into
// groups, each a step of its own.
func (p *parser) seq(closer byte) (*pnode, error) {
	// cuts are where dots split the steps into groups.
	var cuts []int
	var steps []*pnode
	for {
		c := p.peek()
		if c == 0 || c == ',' || c == closer || c == ']' || c == '>' || c == ')' {
			break
		}
		switch c {
		case '.':
			p.pos++
			cuts = append(cuts, len(steps))
			continue
		case '_':
			p.pos++
			if len(steps) == 0 {
				return nil, p.errorf("_ with nothing to hold")
			}
			steps[len(steps)-1].weight++
			continue
		case '!':
			p.pos++
			if len(steps) == 0 {
				return nil, p.errorf("! with nothing to repeat")
			}
			steps = append(steps, steps[len(steps)-1])
			continue
		}
		s, err := p.step()
		if err != nil {
			return nil, err
		}
		steps = append(steps, s...)
	}
	if cuts != nil {
		cuts = append(cuts, len(steps))
		grouped := make([]*pnode, 0, len(cuts))
		from := 0
		for _, to := range cuts {
			if to > from {
				grouped = append(grouped, &pnode{kind: pSeq, kids: steps[from:to], weight: 1})
			}
			from = to
		}
		steps = grouped
	}
	if len(steps) == 1 {
		s := *steps[0]
		s.weight = 1
		return &s, nil
	}
	return &pnode{kind: pSeq, kids: steps, weight: 1}, nil
}

// step reads a term and its modifiers. A replication, as a!3, makes
// several steps.
func (p *parser) step() ([]*pnode, error) {
	n, terr := p.term()
	if terr != nil {
		return nil, terr
	}
	reps := 1
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '*', '/':
			op := p.src[p.pos]
			p.pos++
			k, err := p.int()
			if err != nil {
				return nil, err
			}
			if k < 1 {
				return nil, p.errorf("%c needs a count of 1 or more", op)
			}
			kind := pFast
			if op == '/' {
				kind = pSlow
			}
			n = &pnode{kind: kind, n: k, kids: []*pnode{n}, weight: n.weight}
		case '@':
			p.pos++
			w, err := p.number()
			if err != nil {
				return nil, err
			}
			n.weight = w
		case '!':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] < '0' || p.src[p.pos+1] > '9' {
				return repeat(n, reps), nil
			}
			p.pos++
			k, err := p.int()
			if err != nil {
				return nil, err
			}
			reps = max(1, k)
		case '?':
			p.pos++
			chance := 0.5
			if p.pos < len(p.src) && (p.src[p.pos] == '0' || p.src[p.pos] == '.') {
				var err error
				if chance, err = p.number(); err != nil {
					return nil, err
				}
			}
			p.salt++
			n = &pnode{kind: pDegrade, chance: chance, salt: p.salt * 2654435761, kids: []*pnode{n}, weight: n.weight}
		case '(':
			p.pos++
			args := make([]int, 0, 3)
			for {
				p.skip()
				k, err := p.int()
				if err != nil {
					return nil, err
				}
				args = append(args, k)
				if p.peek() == ',' {
					p.pos++
					continue
				}
				if p.peek() != ')' {
					return nil, p.errorf("a Euclidean rhythm ends with )")
				}
				p.pos++
				break
			}
			if len(args) < 2 || len(args) > 3 || args[1] < 1 {
				return nil, p.errorf("a Euclidean rhythm is (hits,steps) or (hits,steps,rotation)")
			}
			hits := bjorklund(args[0], args[1])
			if len(args) == 3 {
				r := ((args[2] % len(hits)) + len(hits)) % len(hits)
				hits = append(hits[r:], hits[:r]...)
			}
			n = &pnode{kind: pEuclid, hits: hits, kids: []*pnode{n}, weight: n.weight}
		default:
			return repeat(n, reps), nil
		}
	}
	return repeat(n, reps), nil
}

func repeat(n *pnode, k int) []*pnode {
	out := make([]*pnode, k)
	for i := range out {
		out[i] = n
	}
	return out
}

// term reads an atom, a rest, or a bracketed group.
func (p *parser) term() (*pnode, error) {
	c := p.peek()
	switch c {
	case '~':
		p.pos++
		return &pnode{kind: pRest, weight: 1}, nil
	case '[':
		p.pos++
		n, err := p.list(']')
		if err != nil {
			return nil, err
		}
		if p.peek() != ']' {
			return nil, p.errorf("[ needs its ]")
		}
		p.pos++
		n.weight = 1
		return n, nil
	case '<':
		p.pos++
		n, err := p.list('>')
		if err != nil {
			return nil, err
		}
		if p.peek() != '>' {
			return nil, p.errorf("< needs its >")
		}
		p.pos++
		return alternate(n), nil
	}
	start := p.pos
	for p.pos < len(p.src) && isWord(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return nil, p.errorf("unexpected %q", c)
	}
	p.pat.atoms = append(p.pat.atoms, p.src[start:p.pos])
	return &pnode{kind: pAtom, atom: len(p.pat.atoms) - 1, weight: 1}, nil
}

// alternate turns what <> held into alternation: a sequence's steps
// take turns, cycle by cycle, and each sequence of a stack takes turns
// of its own.
func alternate(n *pnode) *pnode {
	switch n.kind {
	case pSeq:
		return &pnode{kind: pAlt, kids: n.kids, weight: 1}
	case pStack:
		kids := make([]*pnode, len(n.kids))
		for i, k := range n.kids {
			kids[i] = alternate(k)
		}
		return &pnode{kind: pStack, kids: kids, weight: 1}
	default:
		c := *n
		c.weight = 1
		return &c
	}
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '.' || c == ':' || c == '#' || c == '-' || c == '+' || c == '\'' || c == '^'
}

func (p *parser) int() (int, error) {
	start := p.pos
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	k, err := strconv.Atoi(p.src[start:p.pos])
	if err != nil {
		return 0, p.errorf("a whole number")
	}
	return k, nil
}

func (p *parser) number() (float64, error) {
	start := p.pos
	for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
		p.pos++
	}
	v, err := strconv.ParseFloat(p.src[start:p.pos], 64)
	if err != nil {
		return 0, p.errorf("a number")
	}
	return v, nil
}

// bjorklund returns k hits spread as evenly as they go over n steps, as
// Bjorklund's algorithm spreads them: (3,8) is x..x..x.
func bjorklund(k, n int) []bool {
	out := make([]bool, n)
	switch {
	case k <= 0:
		return out
	case k >= n:
		for i := range out {
			out[i] = true
		}
		return out
	}
	a := make([][]bool, k)
	for i := range a {
		a[i] = []bool{true}
	}
	b := make([][]bool, n-k)
	for i := range b {
		b[i] = []bool{false}
	}
	for len(b) > 1 {
		m := min(len(a), len(b))
		na := make([][]bool, m)
		for i := range m {
			na[i] = append(append([]bool{}, a[i]...), b[i]...)
		}
		if len(a) > m {
			b = a[m:]
		} else {
			b = b[m:]
		}
		a = na
	}
	flat := make([]bool, 0, n)
	for _, s := range a {
		flat = append(flat, s...)
	}
	for _, s := range b {
		flat = append(flat, s...)
	}
	return flat
}

// query appends the events of cycle to out, in order of where they
// start.
func (p *pattern) query(cycle int, out []pev) []pev {
	out = query(p.root, cycle, 0, 1, out)
	sortEvents(out)
	return out
}

func query(n *pnode, cycle int, start, dur float64, out []pev) []pev {
	switch n.kind {
	case pAtom:
		return append(out, pev{at: start, dur: dur, atom: n.atom})
	case pRest:
		return out
	case pSeq:
		total := 0.0
		for _, k := range n.kids {
			total += k.weight
		}
		t := start
		for _, k := range n.kids {
			d := dur * k.weight / total
			out = query(k, cycle, t, d, out)
			t += d
		}
		return out
	case pStack:
		for _, k := range n.kids {
			out = query(k, cycle, start, dur, out)
		}
		return out
	case pAlt:
		l := len(n.kids)
		return query(n.kids[mod(cycle, l)], floorDiv(cycle, l), start, dur, out)
	case pFast:
		d := dur / float64(n.n)
		for i := range n.n {
			out = query(n.kids[0], cycle*n.n+i, start+float64(i)*d, d, out)
		}
		return out
	case pSlow:
		from := len(out)
		off := float64(mod(cycle, n.n))
		out = query(n.kids[0], floorDiv(cycle, n.n), start-off*dur, dur*float64(n.n), out)
		keep := out[:from]
		for _, e := range out[from:] {
			if e.at >= start-1e-9 && e.at < start+dur-1e-9 {
				keep = append(keep, e)
			}
		}
		return keep
	case pEuclid:
		d := dur / float64(len(n.hits))
		for i, hit := range n.hits {
			if hit {
				out = query(n.kids[0], cycle, start+float64(i)*d, d, out)
			}
		}
		return out
	case pDegrade:
		from := len(out)
		out = query(n.kids[0], cycle, start, dur, out)
		keep := out[:from]
		for _, e := range out[from:] {
			if chance(n.salt, cycle, e.at) >= n.chance {
				keep = append(keep, e)
			}
		}
		return keep
	}
	return out
}

// chance returns a number from 0 to 1 fixed by salt, cycle and at, so a
// degraded pattern drops the same events each time a cycle plays.
func chance(salt uint32, cycle int, at float64) float64 {
	x := uint64(salt) ^ uint64(cycle)*0x9e3779b97f4a7c15 ^ uint64(math.Round(at*1e6))*0xbf58476d1ce4e5b9
	x ^= x >> 31
	x *= 0x94d049bb133111eb
	x ^= x >> 29
	return float64(x>>11) / (1 << 53)
}

// sortEvents sorts events by start, keeping the order of those that
// start together. Patterns are small, so an insertion sort does.
func sortEvents(es []pev) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].at < es[j-1].at-1e-12; j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
}

func mod(a, b int) int { return ((a % b) + b) % b }

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// String returns the pattern as written.
func (p *pattern) String() string { return strings.TrimSpace(p.src) }
