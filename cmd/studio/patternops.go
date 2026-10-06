package main

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"

	"github.com/marrasen/gunim-music/synth"
)

// PatternOp changes a step of a track's pattern, as the pattern editor
// asks: At is the step's path, its kids' indices from the top, split by
// slashes, "" for the whole; Op is what to do, and Arg what with.
//
//	set      make the step the atom Arg, or a rest for ~
//	split    make it Arg copies of itself, in its span
//	after    add a copy after it
//	delete   take it out
//	alt      make it take turns with a copy, <it it>
//	layer    make it play with a copy at once, [it, it]
//	fast     make it Arg times faster, or slower for a negative Arg
//	euclid   spread it Euclid's way, or change the hits by Arg
//	steps    change a Euclid's steps by Arg
//	maybe    make it play half the time, or always again
//	weight   make it a step longer, Arg 1, taking the time from the
//	         step after it, or a step shorter, Arg -1, leaving a rest
type PatternOp struct {
	Track, At, Op, Arg string
}

func init() { gunim.RegisterType[PatternOp]("studio.pattern.op") }

// errNoStep is returned for a step a path leads to none of.
var errNoStep = errors.New("no such step")

// applyOp returns the pattern src changed by op, and the path of the
// step to show chosen after it.
func applyOp(src string, op PatternOp) (out, at string, err error) {
	root, err := synth.ParsePattern(src)
	if err != nil {
		return "", "", err
	}
	// The top is held in a holder, so it can be changed like any step.
	holder := &synth.Step{Kind: synth.StepSeq, Kids: []*synth.Step{root}, Weight: 1}
	path := parsePath(op.At)
	parent, i := holder, 0
	for _, k := range path {
		parent = parent.Kids[i]
		if k < 0 || k >= len(parent.Kids) {
			return "", "", errNoStep
		}
		i = k
	}
	s := parent.Kids[i]
	at = op.At
	switch op.Op {
	case "set":
		w := s.Weight
		atom := strings.TrimSpace(op.Arg)
		switch atom {
		case "", "~":
			parent.Kids[i] = &synth.Step{Kind: synth.StepRest, Weight: w}
		default:
			if strings.ContainsAny(atom, " []<>,") {
				// A bit of pattern of its own, as [bd sn].
				sub, err := synth.ParsePattern(atom)
				if err != nil {
					return "", "", err
				}
				sub.Weight = w
				parent.Kids[i] = sub
			} else {
				parent.Kids[i] = &synth.Step{Kind: synth.StepAtom, Atom: atom, Weight: w}
			}
		}
	case "split":
		n := atoi(op.Arg, 2)
		kids := make([]*synth.Step, n)
		for j := range kids {
			kids[j] = clone(s)
			kids[j].Weight = 1
		}
		parent.Kids[i] = &synth.Step{Kind: synth.StepSeq, Kids: kids, Weight: s.Weight}
		at = join(path, 0)
	case "after":
		if parent.Kind == synth.StepSeq || parent.Kind == synth.StepAlt {
			parent.Kids = slices.Insert(parent.Kids, i+1, clone(s))
			at = replaceLast(path, i+1)
			break
		}
		c := clone(s)
		parent.Kids[i] = &synth.Step{Kind: synth.StepSeq, Kids: []*synth.Step{s, c}, Weight: s.Weight}
		s.Weight, c.Weight = 1, 1
		at = join(path, 1)
	case "delete":
		switch {
		case parent == holder:
			holder.Kids[0] = &synth.Step{Kind: synth.StepRest, Weight: 1}
			at = ""
		case len(parent.Kids) > 1 && (parent.Kind == synth.StepSeq || parent.Kind == synth.StepStack || parent.Kind == synth.StepAlt):
			parent.Kids = slices.Delete(parent.Kids, i, i+1)
			at = replaceLast(path, max(i-1, 0))
		default:
			// The last step of a sequence, or a modifier's step, rests.
			parent.Kids[i] = &synth.Step{Kind: synth.StepRest, Weight: s.Weight}
		}
	case "alt":
		c := clone(s)
		c.Weight = 1
		w := s.Weight
		s.Weight = 1
		parent.Kids[i] = &synth.Step{Kind: synth.StepAlt, Kids: []*synth.Step{s, c}, Weight: w}
		at = join(path, 1)
	case "layer":
		c := clone(s)
		c.Weight = 1
		w := s.Weight
		s.Weight = 1
		parent.Kids[i] = &synth.Step{Kind: synth.StepStack, Kids: []*synth.Step{s, c}, Weight: w}
		at = join(path, 1)
	case "fast":
		d := atoi(op.Arg, 2)
		n := 1
		kind := synth.StepFast
		inner := s
		switch s.Kind {
		case synth.StepFast:
			n, inner = s.N, s.Kids[0]
		case synth.StepSlow:
			n, inner = -s.N, s.Kids[0]
		default:
		}
		// Faster counts up from 1, slower down from -1, with no 0.
		if n == 1 && d < 0 {
			n = -1
		} else if n == -1 && d > 0 {
			n = 1
		}
		if d > 0 {
			n = step(n, 1)
		} else {
			n = step(n, -1)
		}
		w := s.Weight
		switch {
		case n == 1 || n == -1:
			inner.Weight = w
			parent.Kids[i] = inner
		case n < 0:
			kind = synth.StepSlow
			parent.Kids[i] = &synth.Step{Kind: kind, N: -n, Kids: []*synth.Step{inner}, Weight: w}
		default:
			parent.Kids[i] = &synth.Step{Kind: kind, N: n, Kids: []*synth.Step{inner}, Weight: w}
		}
		inner.Weight = max(inner.Weight, 1)
	case "euclid":
		if s.Kind == synth.StepEuclid {
			s.Hits = min(max(s.Hits+atoi(op.Arg, 1), 1), s.Steps)
			break
		}
		w := s.Weight
		s.Weight = 1
		parent.Kids[i] = &synth.Step{Kind: synth.StepEuclid, Hits: 3, Steps: 8, Kids: []*synth.Step{s}, Weight: w}
	case "steps":
		if s.Kind == synth.StepEuclid {
			s.Steps = min(max(s.Steps+atoi(op.Arg, 1), 1), 32)
			s.Hits = min(s.Hits, s.Steps)
		}
	case "maybe":
		if s.Kind == synth.StepDegrade {
			inner := s.Kids[0]
			inner.Weight = s.Weight
			parent.Kids[i] = inner
			break
		}
		w := s.Weight
		s.Weight = 1
		parent.Kids[i] = &synth.Step{Kind: synth.StepDegrade, Chance: 0.5, Kids: []*synth.Step{s}, Weight: w}
	case "weight":
		// A step longer takes its time from the step after it, and a step
		// shorter gives it back as a rest, so the steps around it keep
		// their places in the bar.
		if parent.Kind != synth.StepSeq || parent == holder {
			return "", "", errors.New("only a step of a sequence grows or shrinks")
		}
		if atoi(op.Arg, 1) > 0 {
			if i+1 >= len(parent.Kids) {
				return "", "", errors.New("there is no step after it to take the time from")
			}
			next := parent.Kids[i+1]
			if next.Weight > 1 {
				next.Weight--
			} else {
				parent.Kids = slices.Delete(parent.Kids, i+1, i+2)
			}
			s.Weight++
			break
		}
		if s.Weight <= 1 {
			return "", "", errors.New("it is a single step, as short as it goes; split it to make it shorter")
		}
		s.Weight--
		if i+1 < len(parent.Kids) && parent.Kids[i+1].Kind == synth.StepRest {
			parent.Kids[i+1].Weight++
		} else {
			parent.Kids = slices.Insert(parent.Kids, i+1, &synth.Step{Kind: synth.StepRest, Weight: 1})
		}
	default:
		return "", "", errors.New("no such change: " + op.Op)
	}
	out = holder.Kids[0].String()
	if _, err := synth.ParsePattern(out); err != nil {
		return "", "", err
	}
	return out, at, nil
}

// step moves n by d, skipping 0 and ±1's other side.
func step(n, d int) int {
	n += d
	if n == 0 {
		n += d
	}
	return n
}

func clone(s *synth.Step) *synth.Step {
	c := *s
	c.Kids = make([]*synth.Step, len(s.Kids))
	for i, k := range s.Kids {
		c.Kids[i] = clone(k)
	}
	return &c
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// parsePath reads a step's path: its kids' indices, split by slashes.
func parsePath(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, "/") {
		out = append(out, atoi(p, 0))
	}
	return out
}

func writePath(p []int) string {
	parts := make([]string, len(p))
	for i, k := range p {
		parts[i] = strconv.Itoa(k)
	}
	return strings.Join(parts, "/")
}

func join(p []int, k int) string { return writePath(append(slices.Clone(p), k)) }

func replaceLast(p []int, k int) string {
	if len(p) == 0 {
		return ""
	}
	q := slices.Clone(p)
	q[len(q)-1] = k
	return writePath(q)
}

// stepAt returns the step at path in root, or nil.
func stepAt(root *synth.Step, path []int) *synth.Step {
	s := root
	for _, k := range path {
		if s == nil || k < 0 || k >= len(s.Kids) {
			return nil
		}
		s = s.Kids[k]
	}
	return s
}

// gridOf lays the events of cycle of a pattern out on a grid of cols
// steps: each row a value, as bd or 2, each cell the value's note
// starting there, held on through the cells after it marked hold.
type gridOf struct {
	rows  []string
	cells [][]int8 // 0 empty, 1 a note starts, 2 a note holds on
}

// readGrid returns the events of cycle of src on cols steps, rows first
// those given, then any others the pattern plays.
func readGrid(src string, cycle, cols int, rows []string) gridOf {
	g := gridOf{rows: slices.Clone(rows)}
	evs, _ := synth.PatternEvents(src, cycle)
	for _, e := range evs {
		if !slices.Contains(g.rows, e.Atom) {
			g.rows = append(g.rows, e.Atom)
		}
	}
	g.cells = make([][]int8, len(g.rows))
	for i := range g.cells {
		g.cells[i] = make([]int8, cols)
	}
	for _, e := range evs {
		r := slices.Index(g.rows, e.Atom)
		c := int(math.Round(e.At * float64(cols)))
		if c >= cols {
			continue
		}
		g.cells[r][c] = 1
		end := int(math.Round((e.At + e.Dur) * float64(cols)))
		for h := c + 1; h < min(end, cols); h++ {
			if g.cells[r][h] == 0 {
				g.cells[r][h] = 2
			}
		}
	}
	return g
}

// writeGrid writes cycles' grids back as a pattern: a stack of a row a
// value, each row its steps, a hold as _; several cycles take turns.
func writeGrid(grids []gridOf) string {
	var cycles []string
	for _, g := range grids {
		var rows []string
		for r, name := range g.rows {
			cells := g.cells[r]
			if !slices.ContainsFunc(cells, func(c int8) bool { return c == 1 }) {
				continue
			}
			rows = append(rows, writeRow(name, cells))
		}
		switch len(rows) {
		case 0:
			cycles = append(cycles, "~")
		case 1:
			cycles = append(cycles, rows[0])
		default:
			cycles = append(cycles, "["+strings.Join(rows, ", ")+"]")
		}
	}
	if len(cycles) == 1 {
		c := cycles[0]
		if strings.HasPrefix(c, "[") && strings.HasSuffix(c, "]") && strings.Contains(c, ",") {
			return c[1 : len(c)-1]
		}
		return c
	}
	return "<" + strings.Join(cycles, " ") + ">"
}

// writeRow writes a row's steps, as short as it goes: a pattern of
// steps that repeats is written once, times how often.
func writeRow(name string, cells []int8) string {
	word := func(c int8) string {
		switch c {
		case 1:
			return name
		case 2:
			return "_"
		}
		return "~"
	}
	n := len(cells)
	for period := 1; period < n; period++ {
		if n%period != 0 {
			continue
		}
		repeats := true
		for i := period; i < n && repeats; i++ {
			repeats = cells[i] == cells[i%period]
		}
		if !repeats {
			continue
		}
		// A note held all its period is the note alone, as bd*4.
		held := cells[0] == 1
		for _, c := range cells[1:period] {
			held = held && c == 2
		}
		if held {
			return name + "*" + strconv.Itoa(n/period)
		}
		parts := make([]string, period)
		for i := range parts {
			parts[i] = word(cells[i])
		}
		// A hold may not start a group.
		if cells[0] == 2 {
			break
		}
		body := strings.Join(parts, " ")
		if period > 1 {
			body = "[" + body + "]"
		}
		return body + "*" + strconv.Itoa(n/period)
	}
	parts := make([]string, n)
	for i, c := range cells {
		parts[i] = word(c)
	}
	if cells[0] == 2 {
		parts[0] = "~"
	}
	return "[" + strings.Join(parts, " ") + "]"
}
