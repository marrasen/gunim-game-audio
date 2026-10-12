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

	"github.com/marrasen/gunim-game-audio/synth"
)

// beatSpans are the bars a page of the beat shows, and beatSteps the
// steps a bar may have.
var (
	beatSpans = []int{1, 2, 4, 8}
	beatSteps = []int{8, 16, 32}
)

// beatPane shows every drum track at once, as a drum machine does: a
// lane a track, a row a drum it plays, on one time line, a page of bars
// at a time. A click sets a hit or clears one, and a drag paints them;
// each edit writes its track's pattern anew.
type beatPane struct {
	*widget.Scroll
	grid   *beatGrid
	page   *widget.Label
	span   *widget.Segmented
	res    *widget.Segmented
	follow *widget.Button
	// toTrack and drum add a row for a drum to a track's lane.
	toTrack *widget.Dropdown
	drum    *widget.Dropdown
	tracks  []string
	drums   []string
}

func newBeatPane() *beatPane {
	bp := &beatPane{grid: &beatGrid{perBar: 16, span: 2, following: true}}
	prev := widget.NewIconButton(icon.ChevronLeft, "The bars before")
	prev.OnClick = func(u *gunim.UI) gunim.Intent { bp.grid.turn(-1); u.Invalidate(); return nil }
	next := widget.NewIconButton(icon.ChevronRight, "The bars after")
	next.OnClick = func(u *gunim.UI) gunim.Intent { bp.grid.turn(1); u.Invalidate(); return nil }
	bp.page = small("")
	bp.follow = widget.NewButton("Follow")
	bp.follow.KeepFocus, bp.follow.Tooltip = true, "Turn the page with the song as it plays"
	bp.follow.OnClick = func(u *gunim.UI) gunim.Intent {
		bp.grid.following = !bp.grid.following
		u.Invalidate()
		return nil
	}
	spans := make([]string, len(beatSpans))
	for i, n := range beatSpans {
		spans[i] = strconv.Itoa(n)
	}
	bp.span = widget.NewSegmented(spans...)
	bp.span.KeepFocus = true
	bp.span.SetSelected(1, nil)
	bp.span.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		bp.grid.span = beatSpans[i]
		bp.grid.start -= bp.grid.start % bp.grid.span
		u.Invalidate()
		return nil
	}
	steps := make([]string, len(beatSteps))
	for i, n := range beatSteps {
		steps[i] = strconv.Itoa(n)
	}
	bp.res = widget.NewSegmented(steps...)
	bp.res.KeepFocus = true
	bp.res.SetSelected(1, nil)
	bp.res.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		bp.grid.perBar = beatSteps[i]
		bp.grid.stale = true
		u.Invalidate()
		return nil
	}
	bp.toTrack = widget.NewDropdown(widget.Labels("–"))
	bp.toTrack.Label = "Track"
	bp.drum = widget.NewDropdown(widget.Labels(synth.DrumNames...))
	bp.drum.Label = "Drum"
	add := widget.NewButton("Add row")
	add.Icon, add.Ghost = icon.Plus, true
	add.OnClick = func(u *gunim.UI) gunim.Intent {
		if i, j := bp.toTrack.Selected(), bp.drum.Selected(); i < len(bp.tracks) && j < len(bp.drums) {
			bp.grid.addRow(bp.tracks[i], bp.drums[j])
			u.Invalidate()
		}
		return nil
	}
	nav := widget.Row(prev, widget.NewSized(bp.page, 110, 0), next, bp.follow, widget.NewSpacer(),
		small("Bars"), bp.span, small("Steps a bar"), bp.res)
	nav.Grow(nav.Children()[4], 1)
	nav.Cross = widget.CrossCenter
	hint := small("Every drum track at once. Click a step to set a hit or clear it, or drag to paint; M and S mute and solo.")
	gap := widget.NewSpacer()
	adder := widget.Row(hint, gap, small("Add"), bp.drum, small("to"), bp.toTrack, add).Grow(gap, 1)
	adder.Cross = widget.CrossCenter
	col := widget.Column(nav, adder, bp.grid)
	col.Cross = widget.CrossStretch
	bp.Scroll = widget.NewScroll(widget.NewPad(col))
	return bp
}

// update shows st.
func (bp *beatPane) update(st Studio, u *gunim.UI) {
	song := st.Doc
	if song == nil {
		return
	}
	g := bp.grid
	g.show(st)
	bp.follow.Active = g.following
	bp.page.Text = fmt.Sprintf("bars %d–%d of %d", g.start+1, g.start+g.span, g.loop)
	if g.span == 1 {
		bp.page.Text = fmt.Sprintf("bar %d of %d", g.start+1, g.loop)
	}
	var names []string
	for _, l := range g.lanes {
		names = append(names, l.track)
	}
	if !slices.Equal(names, bp.tracks) {
		bp.tracks = names
		bp.toTrack.SetItems(widget.Labels(names...))
		if len(names) == 0 {
			bp.toTrack.SetItems(widget.Labels("no drum tracks"))
		}
	}
	// The drums of the chosen track's kit, its own names first.
	drums := slices.Clone(synth.DrumNames)
	if i := bp.toTrack.Selected(); i < len(bp.tracks) {
		if t := track(song, bp.tracks[i]); t != nil {
			if p := song.Patches[t.Patch]; p != nil {
				for _, n := range sortedNames(p.Kit) {
					if !slices.Contains(drums, n) {
						drums = append(drums, n)
					}
				}
			}
		}
	}
	if !slices.Equal(drums, bp.drums) {
		bp.drums = drums
		bp.drum.SetItems(widget.Labels(drums...))
	}
	_ = u
}

// beatLane is a drum track as the beat shows it: its pattern read onto a
// grid a cycle, each cycle bars long, a row a drum.
type beatLane struct {
	track      string
	color      color.NRGBA
	src        string
	bars       int
	cycles     int
	perBar     int
	rows       []string
	grids      []gridOf
	mute, solo bool
	// lossy says the grid cannot hold every hit of the pattern, so an
	// edit writes it anew on the grid.
	lossy bool
	// top is where the lane starts down the grid, and muteAt and soloAt
	// its buttons.
	top            float32
	muteAt, soloAt geom.Rect
}

// beatGrid draws the lanes and takes their edits.
type beatGrid struct {
	lanes []*beatLane
	// perBar is the steps a bar has, span the bars a page shows, start
	// the song's bar it starts on, and loop the bars the drums take to
	// come round; stale says the lanes must be read again.
	perBar, span, start, loop int
	stale                     bool
	// following turns the page with the song; playing is the bar heard,
	// with its fraction, -1 where none.
	following bool
	playing   float64
	size      geom.Size
	// The drag: on lane and row, painting paint, last on column last.
	held      bool
	lane, row int
	paint     int8
	last      int
	hover     [3]int
}

const (
	beatHead  = 24
	beatRowH  = 18
	beatGap   = 10
	beatLabel = 92
)

// show reads st's drum tracks into lanes, keeping a lane's rows while its
// track stays, and turns the page with the song where it follows.
func (g *beatGrid) show(st Studio) {
	song := st.Doc
	old := map[string]*beatLane{}
	for _, l := range g.lanes {
		old[l.track] = l
	}
	g.lanes = g.lanes[:0]
	g.loop = 1
	for i, r := range st.Tracks {
		t := track(song, r.Name)
		if t == nil {
			continue
		}
		p := song.Patches[t.Patch]
		if p == nil || p.Kind != "drums" {
			continue
		}
		l, ok := old[r.Name]
		if !ok {
			l = &beatLane{track: r.Name}
		}
		l.color, l.mute, l.solo = hexColor(r.Color, i), r.Mute, r.Solo
		if src := t.Pattern; !ok || g.stale || src != l.src || max(t.Bars, 1) != l.bars || l.perBar != g.perBar {
			l.read(src, max(t.Bars, 1), g.perBar, p)
		}
		g.loop = lcm(g.loop, l.bars*l.cycles)
		g.lanes = append(g.lanes, l)
	}
	g.stale = false
	g.loop = min(g.loop, 512)
	g.playing = -1
	heard := heardAt(st.Clock, st.Clock.At)
	if st.BarFrames > 0 {
		if pos := float64(st.Bar) + (heard-float64(st.BarFrame))/st.BarFrames; pos >= 0 {
			g.playing = math.Mod(pos, float64(g.loop))
		}
	}
	if g.following && g.playing >= 0 && !g.held {
		g.start = int(g.playing) / g.span * g.span
	}
	g.start = ((g.start % g.loop) + g.loop) % g.loop
}

// read reads the pattern src, bars long, onto grids of perBar steps a
// bar, a row a drum it plays, keeping the rows the lane has.
func (l *beatLane) read(src string, bars, perBar int, p *synth.Patch) {
	l.src, l.bars, l.perBar = src, bars, perBar
	l.cycles = synth.PatternCycles(src)
	have := map[string]bool{}
	var rows []string
	add := func(r string) {
		if r != "" && r != "~" && !have[r] {
			have[r] = true
			rows = append(rows, r)
		}
	}
	for _, r := range l.rows {
		add(r)
	}
	for c := range l.cycles {
		evs, _ := synth.PatternEvents(src, c)
		for _, e := range evs {
			add(e.Atom)
		}
	}
	order := map[string]int{}
	for i, d := range synth.DrumNames {
		order[d] = i
	}
	slices.SortStableFunc(rows, func(a, b string) int { return drumRank(order, a) - drumRank(order, b) })
	l.rows = rows
	cols := perBar * bars
	l.grids = make([]gridOf, l.cycles)
	for c := range l.cycles {
		l.grids[c] = readGrid(src, c, cols, rows)
		// A drum is a hit: how long it is held says nothing.
		for _, cells := range l.grids[c].cells {
			for k, v := range cells {
				if v == 2 {
					cells[k] = 0
				}
			}
		}
	}
	l.lossy = !slices.Equal(hitsOf(l.write(), l.cycles), hitsOf(src, l.cycles))
}

// write writes the lane's grids as its pattern.
func (l *beatLane) write() string { return writeGrid(l.grids) }

// hitsOf returns where a pattern's hits start over its cycles, as text,
// to tell whether two patterns hit the same.
func hitsOf(src string, cycles int) []string {
	var out []string
	for c := range cycles {
		evs, _ := synth.PatternEvents(src, c)
		for _, e := range evs {
			out = append(out, fmt.Sprintf("%d %.4f %s", c, e.At, e.Atom))
		}
	}
	slices.Sort(out)
	return out
}

// addRow gives the lane of track a row for drum, empty.
func (g *beatGrid) addRow(track, drum string) {
	for _, l := range g.lanes {
		if l.track != track || slices.Contains(l.rows, drum) {
			continue
		}
		l.rows = append(l.rows, drum)
		for c := range l.grids {
			l.grids[c].rows = l.rows
			l.grids[c].cells = append(l.grids[c].cells, make([]int8, l.perBar*l.bars))
		}
	}
}

// turn turns the page by pages, round the loop, and stops following.
func (g *beatGrid) turn(by int) {
	g.following = false
	g.start = (((g.start + by*g.span) % g.loop) + g.loop) % g.loop
	g.start -= g.start % g.span
}

// at returns where the song's bar b, step s of it, lies in the lane: its
// cycle's grid and the column in it.
func (l *beatLane) at(b, s int) (grid, col int) {
	cyc := b / l.bars
	return cyc % l.cycles, (b%l.bars)*l.perBar + s
}

func (g *beatGrid) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	var h float32
	for _, l := range g.lanes {
		l.top = h
		h += beatHead + float32(len(l.rows))*beatRowH + beatGap
	}
	g.size = c.Constrain(geom.Sz(c.Max.W, max(h, beatHead)))
	return g.size
}

// cols returns the steps a page shows.
func (g *beatGrid) cols() int { return g.span * g.perBar }

// cell returns the lane, row and column of the page at p.
func (g *beatGrid) cell(p geom.Point) (lane, row, col int, ok bool) {
	cw := (g.size.W - beatLabel) / float32(g.cols())
	for i, l := range g.lanes {
		y := p.Y - l.top - beatHead
		if y < 0 || y >= float32(len(l.rows))*beatRowH {
			continue
		}
		col = int((p.X - beatLabel) / cw)
		return i, int(y / beatRowH), col, p.X >= beatLabel && col >= 0 && col < g.cols()
	}
	return 0, 0, 0, false
}

// set sets the cell of lane's row at the page's column col to v.
func (g *beatGrid) set(l *beatLane, row, col int, v int8) {
	b := g.start + col/g.perBar
	gi, c := l.at(b, col%g.perBar)
	if gi < len(l.grids) && row < len(l.grids[gi].cells) && c < len(l.grids[gi].cells[row]) {
		l.grids[gi].cells[row][c] = v
	}
}

// get returns the cell of lane's row at the page's column col.
func (g *beatGrid) get(l *beatLane, row, col int) int8 {
	b := g.start + col/g.perBar
	gi, c := l.at(b, col%g.perBar)
	if gi < len(l.grids) && row < len(l.grids[gi].cells) && c < len(l.grids[gi].cells[row]) {
		return l.grids[gi].cells[row][c]
	}
	return 0
}

func (g *beatGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	cols := g.cols()
	if cols == 0 {
		return
	}
	cw := (box.W - beatLabel) / float32(cols)
	beat := max(g.perBar/4, 1)
	for li, l := range g.lanes {
		y := l.top
		lh := beatHead + float32(len(l.rows))*beatRowH
		p.RRect(geom.Rc(0, y, box.W, lh), 8, paint.Solid(color.NRGBA{0x10, 0x12, 0x19, 0xff}))
		// The lane's head: its colour, name, buttons and length.
		p.RRect(geom.Rc(8, y+8, 8, 8), 4, paint.Solid(l.color))
		name := audioui.Shaped(l.track, 11.5, true, false)
		name.Paint(p, geom.Pt(22, y+(beatHead-name.Height())/2), withAlpha(ink, 0.9))
		x := 22 + name.Advance + 10
		chip := func(label string, on bool, col color.NRGBA) geom.Rect {
			r := geom.Rc(x, y+4, 18, beatHead-8)
			fill := color.NRGBA{0xff, 0xff, 0xff, 0x10}
			if on {
				fill = col
			}
			p.RRect(r, 4, paint.Solid(fill))
			t := audioui.Shaped(label, 10, true, false)
			tc := withAlpha(ink, 0.7)
			if on {
				tc = color.NRGBA{0x12, 0x14, 0x1c, 0xff}
			}
			t.Paint(p, geom.Pt(r.Min.X+(18-t.Advance)/2, r.Min.Y+(r.Max.Y-r.Min.Y-t.Height())/2), tc)
			x += 22
			return r
		}
		l.muteAt = chip("M", l.mute, color.NRGBA{0xff, 0x8a, 0x4c, 0xff})
		l.soloAt = chip("S", l.solo, color.NRGBA{0xff, 0xcf, 0x5c, 0xff})
		info := fmt.Sprintf("%d bar", l.bars)
		if l.bars > 1 {
			info += "s"
		}
		if l.cycles > 1 {
			info += fmt.Sprintf(", %d turns", l.cycles)
		}
		if l.lossy {
			info += " · hits finer than the grid: an edit puts them on it"
		}
		t := audioui.Shaped(info, 10, false, false)
		t.Paint(p, geom.Pt(x+6, y+(beatHead-t.Height())/2), withAlpha(ink, 0.45))
		for r, drum := range l.rows {
			ry := y + beatHead + float32(r)*beatRowH
			if g.hover[0] == li && g.hover[1] == r {
				p.RRect(geom.Rc(0, ry, box.W, beatRowH), 0, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x06}))
			}
			lab := audioui.Shaped(drum, 11, false, true)
			lab.Paint(p, geom.Pt(22, ry+(beatRowH-lab.Height())/2), withAlpha(ink, 0.6))
			for c := range cols {
				cx := beatLabel + float32(c)*cw
				fill := color.NRGBA{0xff, 0xff, 0xff, 0x05}
				if (c/beat)%2 == 0 {
					fill = color.NRGBA{0xff, 0xff, 0xff, 0x0a}
				}
				cell := geom.Rc(cx+0.5, ry+1, cw-1, beatRowH-2)
				if g.get(l, r, c) == 1 {
					p.ShadowRRect(cell, 3, paint.Solid(withAlpha(l.color, 0.92)), paint.Shadow{Blur: 4, Color: withAlpha(l.color, 0.4)})
				} else {
					p.RRect(cell, 2, paint.Solid(fill))
				}
			}
		}
		// Bars, as stronger lines.
		for b := 1; b < g.span; b++ {
			bx := beatLabel + float32(b*g.perBar)*cw
			p.RRect(geom.Rc(bx-0.5, y+beatHead, 1, lh-beatHead), 0, paint.Solid(withAlpha(ink, 0.3)))
		}
	}
	if g.playing >= float64(g.start) && g.playing < float64(g.start+g.span) {
		x := beatLabel + (box.W-beatLabel)*float32((g.playing-float64(g.start))/float64(g.span))
		p.ShadowRRect(geom.Rc(x-1, 0, 2, box.H), 1, paint.Solid(withAlpha(ink, 0.85)), paint.Shadow{Blur: 8, Color: withAlpha(scopeColor, 0.8)})
	}
}

func (g *beatGrid) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		for _, l := range g.lanes {
			switch {
			case l.muteAt.Contains(e.Pos):
				u.Send(g, ToggleValue{Path: "Tracks/" + l.track + "/Mute"})
				return true
			case l.soloAt.Contains(e.Pos):
				u.Send(g, ToggleValue{Path: "Tracks/" + l.track + "/Solo"})
				return true
			}
		}
		lane, row, col, ok := g.cell(e.Pos)
		if !ok {
			return true
		}
		l := g.lanes[lane]
		// A press on a hit clears it, and the drag clears; on an empty
		// step it sets one, and the drag sets.
		g.held, g.lane, g.row, g.paint, g.last = true, lane, row, 1, col
		if g.get(l, row, col) == 1 {
			g.paint = 0
		}
		g.set(l, row, col, g.paint)
		u.Invalidate()
		return true
	case input.PointerMove:
		lane, row, col, ok := g.cell(e.Pos)
		if ok && (lane != g.hover[0] || row != g.hover[1] || col != g.hover[2]) {
			g.hover = [3]int{lane, row, col}
			u.Invalidate()
		}
		if !g.held {
			return false
		}
		if ok && lane == g.lane && row == g.row {
			// Every step from the last to here, so a quick drag skips
			// none.
			for c := min(col, g.last); c <= max(col, g.last); c++ {
				g.set(g.lanes[lane], row, c, g.paint)
			}
			g.last = col
			u.Invalidate()
		}
		return true
	case input.PointerLeave:
		g.hover = [3]int{-1, -1, -1}
		u.Invalidate()
	case input.PointerUp:
		if !g.held {
			return false
		}
		g.held = false
		l := g.lanes[g.lane]
		text := l.write()
		l.src = text
		l.lossy = false
		u.Send(g, PatternEdited{Track: l.track, Text: text})
		u.Invalidate()
		return true
	}
	return false
}

// DragsTouch says a finger paints hits rather than scrolling.
func (g *beatGrid) DragsTouch() bool { return true }

func lcm(a, b int) int {
	x, y := a, b
	for y != 0 {
		x, y = y, x%y
	}
	return a / x * b
}
