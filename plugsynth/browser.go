package plugsynth

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// browser picks a sound: a search field over two columns, the groups of
// each bank, and the sounds of the group picked. A search lists the
// sounds it finds in every group, under their groups' names, and counts
// them beside each group; a click on a group then scrolls to its
// sounds. A click on a sound plays it, and, where the host lets it, a
// short note of it.
type browser struct {
	anim.Group
	p *plugin
	h plug.Host
	// bank and group are the group the sounds column shows, while no
	// search runs, and query the search.
	bank, group string
	query       string
	field       *widget.TextField
	groups      *groupList
	sounds      *soundList
	gs, ss      *widget.Scroll
	// picked tells the view holding the browser a sound was picked.
	picked func(u *gunim.UI)
}

func newBrowser(p *plugin, h plug.Host) *browser {
	st := p.sound()
	b := &browser{p: p, h: h, bank: st.Bank, group: st.Group}
	b.field = widget.NewTextField()
	b.field.Icon = icon.Search
	b.field.Clearable = true
	b.field.Placeholder = fmt.Sprintf("Search %d sounds", len(all()))
	b.field.OnChange = func(text string, u *gunim.UI) gunim.Intent {
		b.query = strings.TrimSpace(text)
		b.fill(u)
		b.ss.JumpTo(0)
		return nil
	}
	b.groups = &groupList{b: b, hover: -1, pill: anim.NewFloat(0), lit: anim.NewFloat(0), hoverY: anim.NewFloat(0)}
	b.sounds = &soundList{b: b, hover: -1, pill: anim.NewFloat(0), shown: anim.NewFloat(0), lit: anim.NewFloat(0), hoverY: anim.NewFloat(0)}
	b.Add(b.groups.pill, b.groups.lit, b.groups.hoverY, b.sounds.pill, b.sounds.shown, b.sounds.lit, b.sounds.hoverY)
	b.gs, b.ss = widget.NewScroll(b.groups), widget.NewScroll(b.sounds)
	b.fill(nil)
	return b
}

// fill lists the groups and the sounds as the search and the group
// picked have them, and springs the pills to the current sound and its
// group.
func (b *browser) fill(u *gunim.UI) {
	b.groups.fill()
	b.sounds.fill()
	b.glide(u)
}

// glide springs the pills to the group shown and the sound playing.
func (b *browser) glide(u *gunim.UI) {
	sp := anim.Snappy
	if u != nil {
		sp = widget.Quick.Get(u.Theme())
	}
	if i := b.groups.index(b.bank, b.group); i >= 0 {
		b.groups.pill.Animate(float32(i), sp)
	}
	if i := b.sounds.index(b.p.sound()); i >= 0 {
		if b.sounds.shown.Value() < 0.01 {
			b.sounds.pill.Jump(float32(i))
		}
		b.sounds.pill.Animate(float32(i), sp)
		b.sounds.shown.Animate(1, sp)
	} else {
		b.sounds.shown.Animate(0, sp)
	}
	if u != nil {
		u.Invalidate()
	}
}

// follow shows the sound playing, as set elsewhere: its group, while no
// search runs, scrolled to.
func (b *browser) follow(u *gunim.UI) {
	st := b.p.sound()
	if b.query == "" && (st.Bank != b.bank || st.Group != b.group) && st.Bank != "" {
		b.bank, b.group = st.Bank, st.Group
		b.fill(u)
	} else {
		b.glide(u)
	}
	b.reveal(u)
}

// reveal scrolls the columns to the group shown and the sound playing.
func (b *browser) reveal(u *gunim.UI) {
	if u == nil {
		return
	}
	if i := b.groups.index(b.bank, b.group); i >= 0 {
		b.gs.Reveal(b.groups.rowRect(i), u)
	}
	if i := b.sounds.index(b.p.sound()); i >= 0 {
		b.ss.Reveal(b.sounds.rowRect(i), u)
	}
}

// showGroup shows the sounds of a group: while no search runs, the
// group's alone; during one, scrolled to the group's.
func (b *browser) showGroup(bank, group string, u *gunim.UI) {
	if b.query != "" {
		if i := b.sounds.headerIndex(bank, group); i >= 0 {
			b.ss.ScrollTo(b.sounds.rowRect(i).Min.Y, widget.Settle.Get(u.Theme()))
		}
		return
	}
	if bank == b.bank && group == b.group {
		return
	}
	b.bank, b.group = bank, group
	b.fill(u)
	b.ss.JumpTo(0)
}

// pick plays s, as one edit for the host, and plays a note of it.
func (b *browser) pick(s Sound, u *gunim.UI) {
	cur := b.p.sound()
	if cur.Bank == s.Bank && cur.Group == s.Group && cur.Name == s.Name {
		b.audition(s, u)
		return
	}
	b.h.BeginEdit("Pick "+s.Name, u)
	b.p.pick(s)
	b.h.Changed(u)
	b.h.EndEdit(u)
	b.glide(u)
	if b.picked != nil {
		b.picked(u)
	}
	b.audition(s, u)
}

// audition plays a short note of s, where the host lets it: a kick of a
// drum kit, a low A of a bass, and middle C of the rest.
func (b *browser) audition(s Sound, u *gunim.UI) {
	a, ok := b.h.(plug.Auditioner)
	if !ok {
		return
	}
	a.Audition(previewPitch(s), 0.8, 700*time.Millisecond, u)
}

// previewPitch is the note a preview of s plays.
func previewPitch(s Sound) float64 {
	switch {
	case s.patch.Kind == "drums":
		return 36
	case strings.Contains(strings.ToLower(s.Group), "bass"):
		return 45
	}
	return 60
}

// step plays the sound by from the one playing, in the group listed,
// round from the last to the first.
func (b *browser) step(by int, u *gunim.UI) {
	var list []Sound
	for _, r := range b.sounds.rows {
		if !r.header {
			list = append(list, r.s)
		}
	}
	if len(list) == 0 {
		return
	}
	i := b.sounds.index(b.p.sound())
	if i >= 0 {
		// index counts the headers: count the sounds alone.
		n := 0
		for _, r := range b.sounds.rows[:i] {
			if !r.header {
				n++
			}
		}
		i = n
	}
	i = ((i+by)%len(list) + len(list)) % len(list)
	b.pick(list[i], u)
	b.reveal(u)
}

// Children implements [gunim.Composite].
func (b *browser) Children() []gunim.Node { return []gunim.Node{b.field, b.gs, b.ss} }

// The browser's sizes.
const (
	fieldH  = 32
	rowH    = 26
	headH   = 30
	colGap  = 10
	groupsW = 0.42
)

// Layout implements [gunim.Node]: the field across the top, and the
// columns under it.
func (b *browser) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	kids.At(0).Layout(gunim.Tight(geom.Sz(size.W, fieldH)))
	kids.At(0).Place(geom.Point{})
	top := float32(fieldH + colGap)
	h := max(size.H-top, 0)
	gw := float32(int(size.W * groupsW))
	kids.At(1).Layout(gunim.Tight(geom.Sz(gw, h)))
	kids.At(1).Place(geom.Pt(0, top))
	kids.At(2).Layout(gunim.Tight(geom.Sz(max(size.W-gw-colGap, 0), h)))
	kids.At(2).Place(geom.Pt(gw+colGap, top))
	return size
}

// Paint implements [gunim.Node]: the columns in wells.
func (b *browser) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	ink := widget.Ink.Get(f.Theme)
	top := float32(fieldH + colGap)
	gw := float32(int(box.W * groupsW))
	for _, r := range []geom.Rect{geom.Rc(0, top, gw, box.H-top), geom.Rc(gw+colGap, top, box.W-gw-colGap, box.H-top)} {
		p.RRect(r, 10, paint.Solid(audioui.Faded(ink, 0.035)))
	}
	for i := range kids.Len() {
		kids.At(i).Paint(p)
	}
}

// listRow is a row of a column: a bank's or a group's caption, or an
// item.
type listRow struct {
	header      bool
	bank, group string
	label       string
	count       int
	s           Sound
	y, h        float32
}

// groupList is the column of groups, under their banks' captions, each
// with how many sounds it holds, or the search finds in it.
type groupList struct {
	b     *browser
	rows  []listRow
	hover int
	// pill is the row the pill marking the group shown stands on, lit
	// how far the pill under the pointer shows, at hoverY.
	pill, lit, hoverY *anim.Float
	size              geom.Size
}

func (g *groupList) fill() {
	var words []string
	if g.b.query != "" {
		words = strings.Fields(strings.ToLower(g.b.query))
	}
	g.rows = g.rows[:0]
	y := float32(4)
	for _, bank := range Banks {
		g.rows = append(g.rows, listRow{header: true, bank: bank, label: strings.ToUpper(bank), y: y, h: headH})
		y += headH
		for _, s := range all() {
			if s.Bank != bank {
				continue
			}
			n := len(g.rows) - 1
			if g.rows[n].header || g.rows[n].group != s.Group {
				g.rows = append(g.rows, listRow{bank: bank, group: s.Group, label: s.Group, y: y, h: rowH})
				y += rowH
				n++
			}
			if s.matches(words) {
				g.rows[n].count++
			}
		}
	}
}

func (g *groupList) index(bank, group string) int {
	for i, r := range g.rows {
		if !r.header && r.bank == bank && r.group == group {
			return i
		}
	}
	return -1
}

func (g *groupList) rowRect(i int) geom.Rect { return geom.Rc(0, g.rows[i].y, g.size.W, g.rows[i].h) }

// Layout implements [gunim.Node]: as tall as its rows.
func (g *groupList) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	h := float32(8)
	if n := len(g.rows); n > 0 {
		h = g.rows[n-1].y + g.rows[n-1].h + 8
	}
	g.size = geom.Sz(c.Max.W, h)
	return g.size
}

// at returns the row at y, or -1.
func (g *groupList) at(y float32) int {
	for i, r := range g.rows {
		if y >= r.y && y < r.y+r.h {
			return i
		}
	}
	return -1
}

// Handle implements [gunim.Handler]: the pointer lights a group, and a
// click shows its sounds.
func (g *groupList) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		g.setHover(g.at(e.Pos.Y), u)
		return false
	case input.PointerLeave:
		g.setHover(-1, u)
		return false
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := g.at(e.Pos.Y)
		if i < 0 || g.rows[i].header {
			return false
		}
		u.Cue(gunim.CuePress, g)
		g.b.showGroup(g.rows[i].bank, g.rows[i].group, u)
		return true
	}
	return false
}

func (g *groupList) setHover(i int, u *gunim.UI) {
	if i >= 0 && g.rows[i].header {
		i = -1
	}
	if i == g.hover {
		return
	}
	sp := widget.Quick.Get(u.Theme())
	if i >= 0 {
		if g.lit.Value() < 0.05 {
			g.hoverY.Jump(g.rows[i].y)
		}
		g.hoverY.Animate(g.rows[i].y, sp)
	}
	g.hover = i
	g.lit.Animate(onOff(i >= 0), sp)
	u.Invalidate()
}

// Paint implements [gunim.Node].
func (g *groupList) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	vis, culled := p.Visible()
	inset := float32(6)
	if a := g.lit.Value(); a > 0.01 {
		p.RRect(geom.Rc(inset, g.hoverY.Value(), box.W-2*inset, rowH), 7, paint.Solid(audioui.Faded(ink, 0.06*a)))
	}
	if g.b.query == "" || g.index(g.b.bank, g.b.group) >= 0 {
		y := rowY(g.rows, g.pill.Value())
		p.RRect(geom.Rc(inset, y, box.W-2*inset, rowH), 7, paint.Solid(audioui.Faded(accent, 0.2)))
		p.RRect(geom.Rc(inset, y+6, 3, rowH-12), 1.5, paint.Solid(accent))
	}
	st := g.b.p.sound()
	for i, r := range g.rows {
		if culled && (r.y+r.h < vis.Min.Y || r.y > vis.Max.Y) {
			continue
		}
		if r.header {
			bankIcon(p, f, r.bank, geom.Rc(14, r.y+11, 12, 12), audioui.Faded(ink, 0.45))
			audioui.Shaped(r.label, 9, true, false).Paint(p, geom.Pt(32, r.y+12), audioui.Faded(ink, 0.45))
			continue
		}
		c := audioui.Faded(ink, 0.82)
		count := audioui.Faded(ink, 0.35)
		if g.b.query != "" && r.count == 0 {
			c, count = audioui.Faded(ink, 0.3), audioui.Faded(ink, 0.18)
		}
		if i == g.index(g.b.bank, g.b.group) && g.b.query == "" {
			c = ink
		}
		num := audioui.Shaped(strconv.Itoa(r.count), 10, false, true)
		run := audioui.Shaped(r.label, 12, false, false)
		room := box.W - 46 - num.Advance - 10
		run.PaintFaded(p, geom.Pt(20, r.y+(rowH-run.Height())/2), c, room, 18)
		num.Paint(p, geom.Pt(box.W-16-num.Advance, r.y+(rowH-num.Height())/2), count)
		if r.bank == st.Bank && r.group == st.Group {
			// The group of the sound playing has a dot.
			p.RRect(geom.Rc(box.W-26-num.Advance, r.y+rowH/2-2.5, 5, 5), 2.5, paint.Solid(accent))
		}
	}
}

// rowY is the top of the row at the fractional index v, between rows.
func rowY(rows []listRow, v float32) float32 {
	if len(rows) == 0 {
		return 0
	}
	i := min(max(int(v), 0), len(rows)-1)
	j := min(i+1, len(rows)-1)
	t := min(max(v-float32(i), 0), 1)
	return rows[i].y + (rows[j].y-rows[i].y)*t
}

// bankIcon draws the icon of bank in r.
func bankIcon(p *paint.Painter, f gunim.Frame, bank string, r geom.Rect, c color.NRGBA) {
	ic := icon.Library
	switch bank {
	case GeneralMIDI:
		ic = icon.Piano
	case Songs:
		ic = icon.ListMusic
	}
	widget.PaintIcon(p, f.Theme, ic, r, c)
}

// soundList is the column of sounds: the group's, or those a search
// finds, under their groups' captions.
type soundList struct {
	b     *browser
	rows  []listRow
	hover int
	// pill is the row the pill marking the sound playing stands on,
	// shown how far it shows; lit is how far the pill under the
	// pointer shows, at hoverY.
	pill, shown, lit, hoverY *anim.Float
	size                     geom.Size
}

func (l *soundList) fill() {
	l.rows = l.rows[:0]
	y := float32(4)
	add := func(r listRow) {
		r.y = y
		y += r.h
		l.rows = append(l.rows, r)
	}
	if l.b.query == "" {
		add(listRow{header: true, bank: l.b.bank, group: l.b.group, label: l.b.group, h: headH})
		for _, s := range all() {
			if s.Bank == l.b.bank && s.Group == l.b.group {
				add(listRow{s: s, label: s.Name, h: rowH})
			}
		}
		return
	}
	for _, s := range search(l.b.query) {
		if n := len(l.rows); n == 0 || l.rows[n-1].bank != s.Bank || l.rows[n-1].group != s.Group {
			add(listRow{header: true, bank: s.Bank, group: s.Group, label: s.Group, h: headH})
		}
		add(listRow{bank: s.Bank, group: s.Group, s: s, label: s.Name, h: rowH})
	}
}

// index returns the row of the sound st names, or -1.
func (l *soundList) index(st State) int {
	for i, r := range l.rows {
		if !r.header && r.s.Bank == st.Bank && r.s.Group == st.Group && r.s.Name == st.Name {
			return i
		}
	}
	return -1
}

// headerIndex returns the row of a group's caption, or -1.
func (l *soundList) headerIndex(bank, group string) int {
	for i, r := range l.rows {
		if r.header && r.bank == bank && r.group == group {
			return i
		}
	}
	return -1
}

func (l *soundList) rowRect(i int) geom.Rect { return geom.Rc(0, l.rows[i].y, l.size.W, l.rows[i].h) }

// Layout implements [gunim.Node]: as tall as its rows.
func (l *soundList) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	h := float32(8)
	if n := len(l.rows); n > 0 {
		h = l.rows[n-1].y + l.rows[n-1].h + 8
	}
	l.size = geom.Sz(c.Max.W, h)
	return l.size
}

func (l *soundList) at(y float32) int {
	for i, r := range l.rows {
		if y >= r.y && y < r.y+r.h {
			return i
		}
	}
	return -1
}

// Handle implements [gunim.Handler]: the pointer lights a sound, and a
// click plays it.
func (l *soundList) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		l.setHover(l.at(e.Pos.Y), u)
		return false
	case input.PointerLeave:
		l.setHover(-1, u)
		return false
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := l.at(e.Pos.Y)
		if i < 0 || l.rows[i].header {
			return false
		}
		u.Cue(gunim.CuePress, l)
		l.b.pick(l.rows[i].s, u)
		return true
	}
	return false
}

func (l *soundList) setHover(i int, u *gunim.UI) {
	if i >= 0 && l.rows[i].header {
		i = -1
	}
	if i == l.hover {
		return
	}
	sp := widget.Quick.Get(u.Theme())
	if i >= 0 {
		if l.lit.Value() < 0.05 {
			l.hoverY.Jump(l.rows[i].y)
		}
		l.hoverY.Animate(l.rows[i].y, sp)
	}
	l.hover = i
	l.lit.Animate(onOff(i >= 0), sp)
	u.Invalidate()
}

// Paint implements [gunim.Node].
func (l *soundList) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	ink, accent := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme)
	vis, culled := p.Visible()
	inset := float32(6)
	if a := l.lit.Value(); a > 0.01 {
		p.RRect(geom.Rc(inset, l.hoverY.Value(), box.W-2*inset, rowH), 7, paint.Solid(audioui.Faded(ink, 0.06*a)))
	}
	if a := l.shown.Value(); a > 0.01 {
		y := rowY(l.rows, l.pill.Value())
		p.ShadowRRect(geom.Rc(inset, y, box.W-2*inset, rowH), 7, paint.Solid(audioui.Faded(accent, 0.28*a)),
			paint.Shadow{Blur: 10, Color: audioui.Faded(accent, 0.25*a)})
	}
	cur := l.index(l.b.p.sound())
	if len(l.rows) <= 1 && l.b.query != "" {
		run := audioui.Shaped("No sound matches", 12, false, false)
		run.Paint(p, geom.Pt((box.W-run.Advance)/2, 40), audioui.Faded(ink, 0.45))
	}
	for i, r := range l.rows {
		if culled && (r.y+r.h < vis.Min.Y || r.y > vis.Max.Y) {
			continue
		}
		if r.header {
			bankIcon(p, f, r.bank, geom.Rc(14, r.y+11, 12, 12), audioui.Faded(ink, 0.45))
			run := audioui.Shaped(strings.ToUpper(r.label), 9, true, false)
			run.PaintFaded(p, geom.Pt(32, r.y+12), audioui.Faded(ink, 0.5), box.W-44, 18)
			continue
		}
		c := audioui.Faded(ink, 0.85)
		if i == cur {
			c = ink
			widget.PaintIcon(p, f.Theme, icon.AudioLines, geom.Rc(box.W-30, r.y+(rowH-14)/2, 14, 14), accent)
		}
		run := audioui.Shaped(r.label, 12, i == cur, false)
		run.PaintFaded(p, geom.Pt(20, r.y+(rowH-run.Height())/2), c, box.W-58, 18)
		if r.s.patch.Kind == "drums" {
			widget.PaintIcon(p, f.Theme, icon.Drum, geom.Rc(box.W-48, r.y+(rowH-12)/2, 12, 12), audioui.Faded(ink, 0.3))
		}
	}
}

// Access implements [gunim.Accessible]: the sounds read as a list.
func (l *soundList) Access() access.Info {
	return access.Info{Role: access.RoleList, Name: "Sounds"}
}

// onOff is 1 for true and 0 for false.
func onOff(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
