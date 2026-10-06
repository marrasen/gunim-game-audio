package main

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"

	"github.com/marrasen/gunim-music/synth"
)

// stepGrid is a track's notes on a grid of steps, a row a drum or a
// note, a column a step, as a piano roll shows them. Its rows stay while
// the track is shown, every note of the scale and every drum of the kit
// among them, so a row emptied stays to fill again. A click sets a note
// or clears one; a drag from an empty cell draws a note as long as the
// drag, and a drag from a note's start sets how long it lasts.
type stepGrid struct {
	// track is the track shown: its rows start afresh as it changes.
	track  string
	src    string
	cycles int
	cycle  int
	// perBar is how many steps a bar has, and bars how many bars the
	// track's cycle lasts; cols is the steps in all.
	perBar int
	bars   int
	cols   int
	drums  bool
	// rows are the values the rows play, top first, and labels what
	// each row says.
	rows   []string
	labels []string
	grids  []gridOf
	color  color.NRGBA
	// playing is where the song is in the track's cycle, from 0 to 1,
	// and nowCycle which cycle it is.
	playing  float64
	nowCycle int
	lossy    bool
	edit     func(text string, u *gunim.UI)
	size     geom.Size
	// The drag in progress: on row, from the note starting at start,
	// drawing a new note or setting an old one's length; moved says it
	// has left the cell it started in.
	held    bool
	dragRow int
	start   int
	drawing bool
	moved   bool
	hover   [2]int
}

// gridRowH is how tall a row is.
const gridRowH = 18

// gridLabelW is how wide the rows' names are.
const gridLabelW = 70

// Layout makes the grid as tall as its rows.
func (g *stepGrid) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	g.size = geom.Sz(c.Max.W, float32(max(len(g.rows), 1)*gridRowH))
	return c.Constrain(g.size)
}

// set shows the pattern src of track t, which takes cycles to come round,
// its row at index ti of st.
func (g *stepGrid) set(src string, cycles int, t *synth.Track, song *synth.Song, row TrackRow, ti int, st Studio) {
	g.color = hexColor(row.Color, max(ti, 0))
	p := song.Patches[t.Patch]
	g.drums = p != nil && p.Kind == "drums"
	bars := max(t.Bars, 1)
	if g.perBar == 0 {
		g.perBar = 16
		if bars >= 4 {
			g.perBar = 8
		}
	}
	cols := min(g.perBar*bars, 128)
	fresh := t.Name != g.track
	if fresh {
		g.track, g.rows = t.Name, nil
	}
	if fresh || src != g.src || len(g.grids) != cycles || cols != g.cols || g.bars != bars {
		g.src, g.cycles, g.cols, g.bars = src, cycles, cols, bars
		g.cycle = min(g.cycle, cycles-1)
		g.layRows(src, t, song, p)
		g.grids = make([]gridOf, cycles)
		for c := range cycles {
			g.grids[c] = readGrid(src, c, cols, g.rows)
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
}

// layRows lays out the rows: those shown already, those the pattern
// plays, and those the track may play, as the scale's notes over two
// octaves or the kit's drums, in an order that reads.
func (g *stepGrid) layRows(src string, t *synth.Track, song *synth.Song, p *synth.Patch) {
	have := map[string]bool{}
	add := func(r string) {
		if r != "" && r != "~" && !have[r] {
			have[r] = true
			g.rows = append(g.rows, r)
		}
	}
	rows := slices.Clone(g.rows)
	g.rows = nil
	for _, r := range rows {
		add(r)
	}
	for c := range g.cycles {
		evs, _ := synth.PatternEvents(src, c)
		for _, e := range evs {
			add(e.Atom)
		}
	}
	switch {
	case g.drums:
		for _, d := range []string{"bd", "sn", "cp", "rim", "hh", "oh", "lt", "mt", "ht", "cr"} {
			add(d)
		}
		if p != nil {
			for _, d := range sortedNames(p.Kit) {
				add(d)
			}
		}
		order := map[string]int{}
		for i, d := range synth.DrumNames {
			order[d] = i
		}
		slices.SortStableFunc(g.rows, func(a, b string) int { return drumRank(order, a) - drumRank(order, b) })
	default:
		if t.Arp != "" {
			add("x")
		}
		for d := 14; d >= 0; d-- {
			add(strconv.Itoa(d))
		}
		for _, c := range []string{"ch", "c3", "c2", "c1", "c0", "b"} {
			add(c)
		}
		slices.SortStableFunc(g.rows, func(a, b string) int { return rowOrder(b) - rowOrder(a) })
	}
	g.labels = make([]string, len(g.rows))
	oct := t.Octave
	if oct == 0 {
		oct = 4
	}
	for i, r := range g.rows {
		g.labels[i] = rowLabel(r, song, oct)
	}
}

// drumRank ranks a drum's row: the kit's usual order, then the rest.
func drumRank(order map[string]int, d string) int {
	if i, ok := order[d]; ok {
		return i
	}
	return 1000
}

// rowLabel says what a row plays: a degree with its note, as 4 · G5.
func rowLabel(atom string, song *synth.Song, oct int) string {
	d, err := strconv.Atoi(atom)
	if err != nil {
		switch atom {
		case "ch":
			return "ch · chord"
		case "b":
			return "b · bass"
		case "x":
			return "x · arp"
		}
		return atom
	}
	n, err := synth.KeyNote(song.Key, song.Scale, d, oct)
	if err != nil {
		return atom
	}
	return atom + " · " + synth.NoteName(n)
}

// addRow adds a row playing atom, where there is none yet.
func (g *stepGrid) addRow(atom string) {
	atom = strings.TrimSpace(atom)
	if atom == "" || slices.Contains(g.rows, atom) {
		return
	}
	g.rows = append([]string{atom}, g.rows...)
	g.labels = append([]string{atom}, g.labels...)
	for c := range g.grids {
		g.grids[c].rows = g.rows
		g.grids[c].cells = append([][]int8{make([]int8, g.cols)}, g.grids[c].cells...)
	}
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

// rowOrder ranks a note's row: higher notes higher, a lifted note an
// octave up, chord tones under the degrees, and the chord and bass at
// the foot.
func rowOrder(a string) int {
	lift := 0
	for strings.HasSuffix(a, "'") {
		a = a[:len(a)-1]
		lift += 70
	}
	base := strings.TrimRight(a, "#b")
	acc := len(a) - len(base)
	switch n, err := strconv.Atoi(base); {
	case err == nil:
		return 1000 + n*10 + acc + lift
	case len(base) > 1 && base[0] == 'c':
		if k, err := strconv.Atoi(base[1:]); err == nil {
			return 500 + k*10 + lift
		}
	case base == "ch":
		return 400
	case base == "b":
		return 100
	case base == "x":
		return 5000
	}
	return 3000 + lift
}

// cell returns the cell at p, and false where p is on none.
func (g *stepGrid) cell(p geom.Point) (row, col int, ok bool) {
	if g.cycle >= len(g.grids) || len(g.rows) == 0 || g.cols == 0 || p.X < gridLabelW {
		return 0, 0, false
	}
	cw := (g.size.W - gridLabelW) / float32(g.cols)
	row, col = int(p.Y/gridRowH), int((p.X-gridLabelW)/cw)
	return row, col, row >= 0 && row < len(g.rows) && col >= 0 && col < g.cols
}

// colAt returns the column under x, held to the grid.
func (g *stepGrid) colAt(x float32) int {
	cw := (g.size.W - gridLabelW) / float32(max(g.cols, 1))
	return min(max(int((x-gridLabelW)/cw), 0), g.cols-1)
}

// noteStart returns where the note holding cell col of cells starts, or
// -1 where none holds it.
func noteStart(cells []int8, col int) int {
	for c := col; c >= 0; c-- {
		switch cells[c] {
		case 1:
			return c
		case 0:
			return -1
		}
	}
	return -1
}

// setLength makes the note starting at start in cells last to end,
// stopping short of the next note.
func setLength(cells []int8, start, end int) {
	cells[start] = 1
	c := start + 1
	for ; c <= end && c < len(cells) && cells[c] != 1; c++ {
		cells[c] = 2
	}
	for ; c < len(cells) && cells[c] == 2; c++ {
		cells[c] = 0
	}
}

func (g *stepGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rc(0, 0, box.W, box.H)
	p.RRect(r, 8, paint.Solid(color.NRGBA{0x10, 0x12, 0x19, 0xff}))
	if g.cycle >= len(g.grids) || len(g.rows) == 0 {
		return
	}
	gr := g.grids[g.cycle]
	ink := audioui.Ink.Get(f.Theme)
	cw := (box.W - gridLabelW) / float32(g.cols)
	beat := max(g.perBar/4, 1)
	for i := range g.rows {
		y := float32(i) * gridRowH
		if i == g.hover[0] {
			p.RRect(geom.Rc(0, y, box.W, gridRowH), 0, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x06}))
		}
		label := g.labels[i]
		c := withAlpha(ink, 0.55)
		if strings.Contains(label, " · ") && !g.drums {
			// The root's rows stand out, as a piano's Cs do.
			if d, err := strconv.Atoi(g.rows[i]); err == nil && d%7 == 0 {
				c = withAlpha(ink, 0.85)
			}
		}
		run := audioui.Shaped(label, 11, false, true)
		run.Paint(p, geom.Pt(8, y+(gridRowH-run.Height())/2), c)
		for col := range g.cols {
			x := gridLabelW + float32(col)*cw
			fill := color.NRGBA{0xff, 0xff, 0xff, 0x05}
			if (col/beat)%2 == 0 {
				fill = color.NRGBA{0xff, 0xff, 0xff, 0x0a}
			}
			p.RRect(geom.Rc(x+0.5, y+1, cw-1, gridRowH-2), 2, paint.Solid(fill))
		}
	}
	// Bars, as stronger lines.
	for b := 1; b < g.bars; b++ {
		x := gridLabelW + float32(b*g.perBar)*cw
		p.RRect(geom.Rc(x-0.5, 0, 1, box.H), 0, paint.Solid(withAlpha(ink, 0.3)))
	}
	// The notes, each a bar from its start to its end, with a grip at
	// its end to drag.
	for i, cells := range gr.cells {
		y := float32(i) * gridRowH
		for col := range cells {
			if cells[col] != 1 {
				continue
			}
			end := col + 1
			for end < len(cells) && cells[end] == 2 {
				end++
			}
			x0 := gridLabelW + float32(col)*cw
			x1 := gridLabelW + float32(end)*cw
			note := geom.Rc(x0+1, y+2, x1-x0-2, gridRowH-4)
			p.ShadowRRect(note, 4, paint.Solid(withAlpha(g.color, 0.9)), paint.Shadow{Blur: 4, Color: withAlpha(g.color, 0.4)})
			if x1-x0 > 8 {
				p.RRect(geom.Rc(x1-5, y+5, 2, gridRowH-10), 1, paint.Solid(withAlpha(ink, 0.5)))
			}
		}
	}
	if g.playing >= 0 && g.nowCycle%max(g.cycles, 1) == g.cycle {
		x := gridLabelW + (box.W-gridLabelW)*float32(g.playing)
		p.ShadowRRect(geom.Rc(x-1, 0, 2, box.H), 1, paint.Solid(withAlpha(ink, 0.85)), paint.Shadow{Blur: 8, Color: withAlpha(scopeColor, 0.8)})
	}
}

func (g *stepGrid) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		row, col, ok := g.cell(e.Pos)
		if !ok {
			return true
		}
		cells := g.grids[g.cycle].cells[row]
		g.held, g.dragRow, g.moved = true, row, false
		if s := noteStart(cells, col); s >= 0 {
			// On a note: a drag sets its length, from its start.
			g.start, g.drawing = s, false
			if cells[col] == 2 {
				// On its tail, the drag starts where the press is.
				setLength(cells, s, col)
				g.moved = true
			}
		} else {
			g.start, g.drawing = col, true
			setLength(cells, col, col)
		}
		u.Invalidate()
		return true
	case input.PointerMove:
		row, col, ok := g.cell(e.Pos)
		if ok && (row != g.hover[0] || col != g.hover[1]) {
			g.hover = [2]int{row, col}
			u.Invalidate()
		}
		if !g.held {
			return false
		}
		end := max(g.colAt(e.Pos.X), g.start)
		cells := g.grids[g.cycle].cells[g.dragRow]
		setLength(cells, g.start, end)
		g.moved = g.moved || end != g.start
		u.Invalidate()
		return true
	case input.PointerUp:
		if !g.held {
			return false
		}
		g.held = false
		cells := g.grids[g.cycle].cells[g.dragRow]
		if !g.drawing && !g.moved {
			// A click on a note takes it out, its tail with it.
			setLength(cells, g.start, g.start)
			cells[g.start] = 0
		}
		text := writeGrid(g.grids)
		g.src = text
		if g.edit != nil {
			g.edit(text, u)
		}
		u.Invalidate()
		return true
	}
	return false
}

// DragsTouch says a finger draws notes rather than scrolling.
func (g *stepGrid) DragsTouch() bool { return true }
