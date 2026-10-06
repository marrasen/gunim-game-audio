package main

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"

	"github.com/marrasen/gunim-music/synth"
)

// patternPane edits a track's pattern by sight: its structure drawn as
// boxes, a step chosen and changed with buttons, and its notes on a grid
// of steps, a cell clicked to set or clear it. Each change writes the
// pattern as text, heard from the next bar.
type patternPane struct {
	*widget.Scroll
	track   string
	tracks  []string
	picker  *widget.Dropdown
	field   *widget.TextField
	err     *widget.Label
	tree    *treeView
	chosen  *widget.Label
	atom    *widget.TextField
	palette []*widget.Button
	ops     []*opButton
	grid    *stepGrid
	cycles  *widget.Segmented
	note    *widget.Label
	shownOf string
	gen     int
	sel     string
	selGen  int
	changed func(track string) gunim.Intent
}

// opButton is a button that changes the step chosen.
type opButton struct {
	*widget.Button
	op, arg string
}

func newPatternPane(changed func(string) gunim.Intent) *patternPane {
	pp := &patternPane{changed: changed, gen: -1, selGen: -1}
	pp.picker = widget.NewDropdown("–")
	pp.picker.Label = "Track"
	pp.picker.OnChange = func(i int) gunim.Intent {
		if i < len(pp.tracks) {
			pp.track = pp.tracks[i]
			pp.gen = -1
			pp.sel = ""
		}
		return pp.changed(pp.track)
	}
	pp.field = widget.NewTextField()
	pp.field.Face = widget.MonoFont
	pp.field.OnChange = func(s string) gunim.Intent { return PatternEdited{Track: pp.track, Text: s} }
	pp.err = small("")
	pp.err.Color = problem
	head := widget.Row(pp.picker, pp.field).Grow(pp.field, 1)
	head.Cross = widget.CrossCenter

	pp.tree = &treeView{}
	pp.tree.choose = func(path string) { pp.choose(path) }
	pp.chosen = widget.NewLabel("")
	pp.chosen.Size = smallSize
	pp.atom = widget.NewTextField()
	pp.atom.Face = widget.MonoFont
	pp.atom.Placeholder = "a value, as bd, 2, c1, ch or x"
	pp.atom.OnSubmit = func(s string) gunim.Intent { return PatternOp{Track: pp.track, At: pp.sel, Op: "set", Arg: s} }
	set := widget.NewIconButton(icon.Check, "Set the step to this")
	set.KeepFocus = true
	pp.palette = make([]*widget.Button, 16)
	pal := make([]gunim.Node, len(pp.palette))
	for i := range pp.palette {
		b := widget.NewButton("·")
		b.KeepFocus, b.Ghost = true, true
		pp.palette[i] = b
		pal[i] = b
	}
	type opDef struct {
		label, op, arg, tip string
		ic                  *icon.Icon
	}
	defs := []opDef{
		{"Rest", "set", "~", "Make it a rest, ~", icon.Minus},
		{"Split 2", "split", "2", "Make it two steps in its span", icon.Columns2},
		{"Split 3", "split", "3", "Make it three steps in its span", icon.Columns3},
		{"Split 4", "split", "4", "Make it four steps in its span", icon.Columns4},
		{"Add after", "after", "", "Add a copy after it", icon.Plus},
		{"Delete", "delete", "", "Take it out", icon.Trash2},
		{"Faster", "fast", "1", "Play it once more in its span, *n", icon.ChevronsRight},
		{"Slower", "fast", "-1", "Play it over more spans, /n", icon.ChevronsLeft},
		{"Take turns", "alt", "", "Make it take turns, a cycle each, <a b>", icon.Repeat},
		{"Layer", "layer", "", "Play a copy with it, [a, b]", icon.Layers},
		{"Euclid", "euclid", "1", "Spread it Euclid's way, (3,8), or add a hit", icon.CircleDot},
		{"Fewer hits", "euclid", "-1", "Take a hit from its Euclidean rhythm", icon.Circle},
		{"More steps", "steps", "1", "Spread its hits over a step more", icon.Plus},
		{"Fewer steps", "steps", "-1", "Spread its hits over a step less", icon.Minus},
		{"Maybe", "maybe", "", "Play it half the time, ?, or always again", icon.Dices},
		{"Longer", "weight", "1", "Give it a bigger share of its sequence, @n", icon.MoveHorizontal},
		{"Shorter", "weight", "-1", "Give it a smaller share of its sequence", icon.Minimize2},
	}
	opNodes := make([]gunim.Node, 0, len(defs))
	for _, d := range defs {
		b := &opButton{Button: widget.NewButton(d.label), op: d.op, arg: d.arg}
		b.Icon, b.Tooltip, b.KeepFocus, b.Ghost = d.ic, d.tip, true, true
		pp.ops = append(pp.ops, b)
		opNodes = append(opNodes, b)
	}
	set.On = nil
	pp.ops = append(pp.ops, &opButton{Button: &set.Button, op: "set"})
	atomRow := widget.Row(pp.chosen, sized(pp.atom, 200, 0), set)
	atomRow.Cross = widget.CrossCenter
	palRow := widget.NewWrap(pal...)
	opsRow := widget.NewWrap(opNodes...)
	structure := panel("STRUCTURE · click a step to choose it, then change it", sized(pp.tree, 0, 170), atomRow, palRow, opsRow)

	pp.grid = &stepGrid{}
	pp.grid.edit = func(text string, u *gunim.UI) {
		u.Send(pp.grid, PatternEdited{Track: pp.track, Text: text})
	}
	pp.cycles = widget.NewSegmented("cycle 1")
	pp.cycles.KeepFocus = true
	pp.cycles.OnChange = func(i int) gunim.Intent {
		pp.grid.cycle = i
		return pp.changed(pp.track)
	}
	pp.note = small("")
	gridHead := widget.Row(small("STEPS · click a cell to set or clear it; Shift and click holds the note before it"), widget.NewSpacer(), pp.cycles)
	gridHead.Grow(gridHead.Children()[1], 1)
	gridHead.Cross = widget.CrossCenter
	grid := panelWith(gridHead, sized(pp.grid, 0, 240), pp.note)

	col := widget.Column(head, pp.err, structure, grid)
	col.Cross = widget.CrossStretch
	pp.Scroll = widget.NewScroll(widget.NewPad(col))
	return pp
}

// choose chooses the step at path.
func (pp *patternPane) choose(path string) {
	pp.sel = path
	pp.tree.sel = path
	for _, b := range pp.ops {
		b.On = PatternOp{Track: pp.track, At: path, Op: b.op, Arg: b.arg}
	}
	s := stepAt(pp.tree.root, parsePath(path))
	if s == nil {
		pp.chosen.SetText("Choose a step")
		return
	}
	pp.chosen.SetText(describe(s))
	if s.Kind == synth.StepAtom {
		pp.atom.SetText(s.Atom)
	} else {
		pp.atom.SetText(s.String())
	}
	pp.atom.Select(0, 0)
}

// describe says what a step is, for a person.
func describe(s *synth.Step) string {
	switch s.Kind {
	case synth.StepAtom:
		return "The step " + s.Atom
	case synth.StepRest:
		return "A rest"
	case synth.StepSeq:
		return fmt.Sprintf("A sequence of %d steps", len(s.Kids))
	case synth.StepStack:
		return fmt.Sprintf("%d layers at once", len(s.Kids))
	case synth.StepAlt:
		return fmt.Sprintf("%d turns, a cycle each", len(s.Kids))
	case synth.StepFast:
		return fmt.Sprintf("Played %d times in its span", s.N)
	case synth.StepSlow:
		return fmt.Sprintf("Played over %d spans", s.N)
	case synth.StepEuclid:
		return fmt.Sprintf("%d hits over %d steps", s.Hits, s.Steps)
	case synth.StepDegrade:
		return "Played half the time"
	}
	return ""
}

func (pp *patternPane) update(st Studio, u *gunim.UI) {
	song := st.Doc
	if song == nil {
		return
	}
	var names []string
	for _, t := range song.Tracks {
		names = append(names, t.Name)
	}
	if !slices.Equal(names, pp.tracks) {
		pp.tracks = names
		pp.picker.Items = names
	}
	if !slices.Contains(names, pp.track) && len(names) > 0 {
		pp.track = names[0]
		pp.gen = -1
	}
	pp.picker.Selected = max(slices.Index(names, pp.track), 0)
	t := track(song, pp.track)
	if t == nil {
		return
	}
	src := t.Pattern
	row := TrackRow{}
	ti := -1
	for i, r := range st.Tracks {
		if r.Name == pp.track {
			row, ti = r, i
		}
	}
	if row.Pattern != "" {
		src = row.Pattern
	}
	if pp.gen != st.Gen {
		pp.gen = st.Gen
		pp.field.SetText(src)
		pp.field.Select(0, 0)
	}
	pp.err.SetText(row.PatternErr)
	if t.Melody != nil {
		pp.err.SetText("This track writes its own melody; its pattern goes unplayed.")
	}
	drums := song.Patches[t.Patch] != nil && song.Patches[t.Patch].Kind == "drums"
	// The palette offers what the track plays.
	var pal []string
	switch {
	case drums:
		pal = synth.DrumNames[:16]
	case t.Arp != "":
		pal = []string{"x", "~", "c0", "c1", "c2", "c3", "0", "2", "4", "7", "ch", "b", "x'", "c0'", "c1'", "c2'"}
	default:
		pal = []string{"~", "0", "1", "2", "3", "4", "5", "6", "7", "c0", "c1", "c2", "c3", "ch", "b", "x"}
	}
	for i, b := range pp.palette {
		b.Label = pal[i%len(pal)]
		b.On = PatternOp{Track: pp.track, At: pp.sel, Op: "set", Arg: b.Label}
	}
	if src != pp.shownOf {
		pp.shownOf = src
		if root, err := synth.ParsePattern(src); err == nil {
			pp.tree.root = root
		}
	}
	if st.PatternSelGen != pp.selGen && st.PatternTrack == pp.track {
		pp.selGen = st.PatternSelGen
		pp.choose(st.PatternSel)
	} else {
		pp.choose(pp.sel)
	}
	bars := max(t.Bars, 1)
	n := synth.PatternCycles(src)
	if len(pp.cycles.Labels) != n {
		labels := make([]string, n)
		for i := range labels {
			labels[i] = "cycle " + strconv.Itoa(i+1)
		}
		pp.cycles.Labels = labels
		pp.grid.cycle = min(pp.grid.cycle, n-1)
		pp.cycles.SetSelected(pp.grid.cycle, u)
	}
	pp.grid.set(src, n, bars, drums, t, row, ti, st)
	// Where the song is in the track's cycle, for the tree's light.
	pp.tree.playing = pp.grid.playing
	pp.tree.cycle = pp.grid.nowCycle
	pp.tree.color = hexColor(row.Color, max(ti, 0))
	if pp.grid.lossy {
		pp.note.SetText("The grid writes the pattern out step by step, a cycle each: what it cannot show, as triplets, it rounds to the nearest step.")
	} else {
		pp.note.SetText("")
	}
}

// treeView draws a pattern's structure as boxes in boxes, a cycle wide:
// a sequence splits its box by its steps' shares, layers and turns
// stack, the turn playing lit, and a modifier marks its box.
type treeView struct {
	root    *synth.Step
	sel     string
	hover   string
	choose  func(path string)
	boxes   []treeBox
	color   color.NRGBA
	cycle   int
	playing float64
	size    geom.Size
}

type treeBox struct {
	r    geom.Rect
	path string
	s    *synth.Step
	// ghost marks a repeat a Fast draws of its step, which shows but
	// takes no click.
	ghost bool
	// live says the box plays in the cycle playing, as one turn of a
	// <> does and the others do not.
	live bool
}

func (tv *treeView) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	tv.size = c.Max
	return c.Max
}

// lay lays out s and what it holds in r, cycle the cycle shown.
func (tv *treeView) lay(s *synth.Step, r geom.Rect, path []int, cycle int, live bool) {
	tv.layBox(s, r, path, cycle, live, false)
}

func (tv *treeView) layBox(s *synth.Step, r geom.Rect, path []int, cycle int, live, ghost bool) {
	tv.boxes = append(tv.boxes, treeBox{r: r, path: writePath(path), s: s, live: live, ghost: ghost})
	// Layers and turns keep a strip for their name; modifiers mark a
	// corner, and hold their step close inside.
	in := geom.Rc(r.Min.X+3, r.Min.Y+14, r.Size().W-6, r.Size().H-17)
	switch s.Kind {
	case synth.StepFast, synth.StepSlow, synth.StepEuclid, synth.StepDegrade:
		in = geom.Rc(r.Min.X+3, r.Min.Y+3, r.Size().W-6, r.Size().H-6)
	default:
	}
	if in.Size().W < 4 || in.Size().H < 6 {
		return
	}
	switch s.Kind {
	case synth.StepSeq:
		in = geom.Rc(r.Min.X+2, r.Min.Y+2, r.Size().W-4, r.Size().H-4)
		if len(path) == 0 {
			in = r
		} else {
			in.Min.Y += 12
		}
		total := 0.0
		for _, k := range s.Kids {
			total += k.Weight
		}
		x := in.Min.X
		for i, k := range s.Kids {
			w := in.Size().W * float32(k.Weight/total)
			tv.layBox(k, geom.Rc(x, in.Min.Y, w, in.Size().H), append(slices.Clone(path), i), cycle, live, ghost)
			x += w
		}
	case synth.StepStack, synth.StepAlt:
		h := in.Size().H / float32(len(s.Kids))
		turn := -1
		if s.Kind == synth.StepAlt {
			turn = ((cycle % len(s.Kids)) + len(s.Kids)) % len(s.Kids)
		}
		for i, k := range s.Kids {
			kc := cycle
			if s.Kind == synth.StepAlt {
				kc = cycle / len(s.Kids)
			}
			tv.layBox(k, geom.Rc(in.Min.X, in.Min.Y+float32(i)*h, in.Size().W, h), append(slices.Clone(path), i), kc, live && (turn < 0 || turn == i), ghost)
		}
	case synth.StepFast:
		// Its repeats, the first its step itself and the rest ghosts.
		n := max(min(s.N, 32), 1)
		w := in.Size().W / float32(n)
		for i := range n {
			tv.layBox(s.Kids[0], geom.Rc(in.Min.X+float32(i)*w, in.Min.Y, w, in.Size().H), append(slices.Clone(path), 0), cycle*n+i, live, ghost || i > 0)
		}
	case synth.StepSlow, synth.StepEuclid, synth.StepDegrade:
		if len(s.Kids) > 0 {
			tv.layBox(s.Kids[0], in, append(slices.Clone(path), 0), cycle, live, ghost)
		}
	default:
	}
}

func (tv *treeView) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, 8, paint.Solid(color.NRGBA{0x10, 0x12, 0x19, 0xff}))
	tv.boxes = tv.boxes[:0]
	if tv.root == nil {
		return
	}
	tv.lay(tv.root, geom.Rc(4, 4, box.W-8, box.H-8), nil, tv.cycle, true)
	ink := audioui.Ink.Get(f.Theme)
	accent := widget.Accent.Get(f.Theme)
	for _, b := range tv.boxes {
		s := b.s
		c := tv.color
		if c == (color.NRGBA{}) {
			c = accent
		}
		var fill color.NRGBA
		label := ""
		switch s.Kind {
		case synth.StepAtom:
			fill = withAlpha(c, 0.55)
			label = s.Atom
		case synth.StepRest:
			fill = color.NRGBA{0xff, 0xff, 0xff, 0x08}
			label = "~"
		case synth.StepSeq:
			if b.path == "" {
				continue
			}
			fill = color.NRGBA{0xff, 0xff, 0xff, 0x06}
			label = "[ ]"
		case synth.StepStack:
			fill = color.NRGBA{0x7c, 0x8c, 0xff, 0x22}
			label = "layers"
		case synth.StepAlt:
			fill = color.NRGBA{0xc5, 0x8c, 0xff, 0x22}
			label = "< > turns"
		case synth.StepFast:
			fill = color.NRGBA{0xff, 0xcf, 0x5c, 0x1c}
			label = "×" + strconv.Itoa(s.N)
		case synth.StepSlow:
			fill = color.NRGBA{0xff, 0xcf, 0x5c, 0x1c}
			label = "/" + strconv.Itoa(s.N)
		case synth.StepEuclid:
			fill = color.NRGBA{0x5c, 0xe1, 0xa8, 0x1c}
			label = fmt.Sprintf("(%d,%d)", s.Hits, s.Steps)
		case synth.StepDegrade:
			fill = color.NRGBA{0xff, 0x6b, 0x9d, 0x1c}
			label = "? maybe"
		}
		if !b.live {
			fill.A /= 3
		}
		if b.ghost {
			fill.A = fill.A * 2 / 3
		}
		op := paint.RRectOp{Rect: geom.Rc(b.r.Min.X+1, b.r.Min.Y+1, b.r.Size().W-2, b.r.Size().H-2), Radius: 6, Fill: paint.Solid(fill)}
		if b.path == tv.hover {
			op.Stroke = paint.Stroke{Width: 1, Color: withAlpha(ink, 0.5)}
		}
		if b.path == tv.sel {
			op.Stroke = paint.Stroke{Width: 2, Color: lighten(c, 0.5)}
			op.Shadow = paint.Shadow{Blur: 10, Color: withAlpha(c, 0.6)}
		}
		p.DrawRRect(op)
		if s.Kind == synth.StepEuclid && b.r.Size().W > 40 {
			hits := euclidHits(s.Hits, s.Steps, s.Rotate)
			dw := (b.r.Size().W - 12) / float32(len(hits))
			for i, h := range hits {
				d := min(dw-2, 6)
				col := withAlpha(ink, 0.2)
				if h {
					col = withAlpha(c, 0.9)
				}
				p.RRect(geom.Rc(b.r.Min.X+6+float32(i)*dw, b.r.Max.Y-d-4, d, d), d/2, paint.Solid(col))
			}
		}
		size := float32(12)
		if s.Kind != synth.StepAtom {
			size = 10
		}
		run := audioui.Shaped(label, size, s.Kind == synth.StepAtom, s.Kind == synth.StepAtom)
		switch s.Kind {
		case synth.StepFast, synth.StepSlow, synth.StepEuclid, synth.StepDegrade:
			// A badge in the top right corner.
			if b.r.Size().W > run.Advance+10 {
				badge := geom.Rc(b.r.Max.X-run.Advance-10, b.r.Min.Y-5, run.Advance+8, run.Height()+2)
				p.RRect(badge, 4, paint.Solid(color.NRGBA{0x10, 0x12, 0x19, 0xe8}))
				run.Paint(p, geom.Pt(badge.Min.X+4, badge.Min.Y+1), lighten(fill, 0.6))
			}
		default:
			if run.Advance < b.r.Size().W-6 && run.Height() < b.r.Size().H {
				y := b.r.Min.Y + 2
				if s.Kind == synth.StepAtom || s.Kind == synth.StepRest {
					y = b.r.Min.Y + (b.r.Size().H-run.Height())/2
				}
				tc := withAlpha(ink, 0.9)
				if s.Kind != synth.StepAtom {
					tc = withAlpha(ink, 0.5)
				}
				run.Paint(p, geom.Pt(b.r.Min.X+(b.r.Size().W-run.Advance)/2, y), tc)
			}
		}
	}
	// Where the cycle is, as a line across the top level.
	if tv.playing >= 0 {
		x := 4 + (box.W-8)*float32(tv.playing)
		p.ShadowRRect(geom.Rc(x-1, 2, 2, box.H-4), 1, paint.Solid(withAlpha(ink, 0.8)), paint.Shadow{Blur: 6, Color: withAlpha(scopeColor, 0.8)})
	}
}

// euclidHits returns k hits over n steps, rotated by r, as the pattern
// plays them.
func euclidHits(k, n, r int) []bool {
	evs, err := synth.PatternEvents(fmt.Sprintf("x(%d,%d,%d)", k, max(n, 1), r), 0)
	out := make([]bool, max(n, 1))
	if err != nil {
		return out
	}
	for _, e := range evs {
		if i := int(math.Round(e.At * float64(len(out)))); i < len(out) {
			out[i] = true
		}
	}
	return out
}

// at returns the deepest box at p.
func (tv *treeView) at(p geom.Point) (string, bool) {
	found, ok := "", false
	for _, b := range tv.boxes {
		if !b.ghost && b.r.Contains(p) && (b.path != "" || b.s.Kind != synth.StepSeq) {
			found, ok = b.path, true
		}
	}
	return found, ok
}

func (tv *treeView) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if path, ok := tv.at(e.Pos); ok && tv.choose != nil {
			tv.choose(path)
			u.Invalidate()
		}
		return true
	case input.PointerMove:
		path, _ := tv.at(e.Pos)
		if path != tv.hover {
			tv.hover = path
			u.Invalidate()
		}
	}
	return false
}

// stepGrid is a track's notes on a grid of steps: a row a drum or a
// note, a column a step, the cycle shown's notes lit, and a line where
// the song is.
type stepGrid struct {
	src     string
	cycles  int
	cycle   int
	cols    int
	drums   bool
	grids   []gridOf
	color   color.NRGBA
	playing float64
	// nowCycle is the cycle the song plays of the track.
	nowCycle int
	lossy    bool
	edit     func(text string, u *gunim.UI)
	size     geom.Size
}

// set shows the pattern src, which takes cycles to come round, each
// lasting bars, of track t, its row, at index ti of st.
func (g *stepGrid) set(src string, cycles, bars int, drums bool, t *synth.Track, row TrackRow, ti int, st Studio) {
	g.color = hexColor(row.Color, max(ti, 0))
	g.drums = drums
	perBar := 16
	if bars >= 4 {
		perBar = 8
	}
	g.cols = min(perBar*bars, 64)
	g.cycles = cycles
	g.cycle = min(g.cycle, cycles-1)
	if src != g.src || len(g.grids) != cycles {
		g.src = src
		var rows []string
		if drums {
			rows = []string{"bd", "sn", "cp", "hh", "oh"}
		}
		g.grids = make([]gridOf, cycles)
		all := map[string]bool{}
		for c := range cycles {
			for _, r := range readGrid(src, c, g.cols, rows).rows {
				all[r] = true
			}
		}
		// Every cycle has the same rows, in an order that reads.
		for r := range all {
			if !slices.Contains(rows, r) {
				rows = append(rows, r)
			}
		}
		if !drums {
			slices.SortStableFunc(rows, func(a, b string) int { return rowOrder(b) - rowOrder(a) })
		}
		for c := range cycles {
			g.grids[c] = readGrid(src, c, g.cols, rows)
		}
		g.lossy = !slices.Equal(evsOf(writeGrid(g.grids), cycles), evsOf(src, cycles))
	}
	// Where the song is in the track's cycle.
	g.playing = -1
	if st.BarFrames > 0 {
		heard := heardAt(st.Clock, st.Clock.At)
		pos := float64(st.Bar) + (heard-float64(st.BarFrame))/st.BarFrames
		if pos >= 0 {
			cyc := pos / float64(bars)
			g.nowCycle = int(cyc)
			g.playing = cyc - math.Floor(cyc)
		}
	}
	_ = t
}

// evsOf returns a pattern's events over its cycles, as text, to tell
// whether two patterns play the same.
func evsOf(src string, cycles int) []string {
	var out []string
	for c := range cycles {
		evs, _ := synth.PatternEvents(src, c)
		for _, e := range evs {
			out = append(out, fmt.Sprintf("%d %.4f %.4f %s", c, e.At, e.Dur, e.Atom))
		}
	}
	// Events that start together play the same in any order.
	slices.Sort(out)
	return out
}

// rowOrder ranks a note's row: higher notes higher.
func rowOrder(a string) int {
	n, err := strconv.Atoi(a)
	switch {
	case err == nil:
		return n * 10
	case len(a) > 1 && a[0] == 'c':
		if k, err := strconv.Atoi(a[1:]); err == nil {
			return k*20 + 1
		}
	case a == "ch":
		return 500
	case a == "b":
		return -100
	}
	return 1000
}

func (g *stepGrid) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	g.size = c.Max
	return c.Max
}

const gridLabelW = 54

// cell returns the cell at p, and false where p is on none.
func (g *stepGrid) cell(p geom.Point) (row, col int, ok bool) {
	if g.cycle >= len(g.grids) {
		return 0, 0, false
	}
	gr := g.grids[g.cycle]
	if len(gr.rows) == 0 || g.cols == 0 || p.X < gridLabelW {
		return 0, 0, false
	}
	rh := g.size.H / float32(len(gr.rows))
	cw := (g.size.W - gridLabelW) / float32(g.cols)
	row, col = int(p.Y/rh), int((p.X-gridLabelW)/cw)
	return row, col, row >= 0 && row < len(gr.rows) && col >= 0 && col < g.cols
}

func (g *stepGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, 8, paint.Solid(color.NRGBA{0x10, 0x12, 0x19, 0xff}))
	if g.cycle >= len(g.grids) {
		return
	}
	gr := g.grids[g.cycle]
	if len(gr.rows) == 0 {
		return
	}
	ink := audioui.Ink.Get(f.Theme)
	rh := box.H / float32(len(gr.rows))
	cw := (box.W - gridLabelW) / float32(g.cols)
	beat := max(g.cols/max(g.cols/4, 1), 1)
	perBeat := g.cols / max(beat, 1)
	_ = perBeat
	for i, name := range gr.rows {
		y := float32(i) * rh
		run := audioui.Shaped(name, min(12, rh*0.6), false, true)
		run.Paint(p, geom.Pt(8, y+(rh-run.Height())/2), withAlpha(ink, 0.7))
		for c := range g.cols {
			x := gridLabelW + float32(c)*cw
			cell := geom.Rc(x+1, y+1, cw-2, rh-2)
			base := color.NRGBA{0xff, 0xff, 0xff, 0x06}
			if (c/4)%2 == 0 {
				base = color.NRGBA{0xff, 0xff, 0xff, 0x0b}
			}
			switch gr.cells[i][c] {
			case 1:
				p.RRect(cell, 3, paint.Solid(withAlpha(g.color, 0.85)))
			case 2:
				p.RRect(geom.Rc(x-1, y+rh*0.3, cw, rh*0.4), 2, paint.Solid(withAlpha(g.color, 0.5)))
			default:
				p.RRect(cell, 3, paint.Solid(base))
			}
		}
	}
	if g.playing >= 0 && g.nowCycle%max(g.cycles, 1) == g.cycle {
		x := gridLabelW + (box.W-gridLabelW)*float32(g.playing)
		p.ShadowRRect(geom.Rc(x-1, 0, 2, box.H), 1, paint.Solid(withAlpha(ink, 0.85)), paint.Shadow{Blur: 8, Color: withAlpha(scopeColor, 0.8)})
	}
}

func (g *stepGrid) Handle(e input.Event, u *gunim.UI) bool {
	d, ok := e.(input.PointerDown)
	if !ok {
		return false
	}
	row, col, ok := g.cell(d.Pos)
	if !ok {
		return true
	}
	cells := g.grids[g.cycle].cells[row]
	switch {
	case d.Mods.Has(input.ModShift) && col > 0 && cells[col] == 0 && cells[col-1] != 0:
		cells[col] = 2
	case cells[col] != 0:
		cells[col] = 0
		// A note cleared takes its holds with it.
		for h := col + 1; h < len(cells) && cells[h] == 2; h++ {
			cells[h] = 0
		}
	default:
		cells[col] = 1
	}
	text := writeGrid(g.grids)
	g.src = ""
	if g.edit != nil {
		g.edit(text, u)
	}
	u.Invalidate()
	return true
}
