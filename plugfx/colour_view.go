package plugfx

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audio/plug/plugui"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// colourView returns Colour's view at size s: the chain's units in a
// chip, and in a panel the chain itself, its units' controls on cards to
// add, remove and drag into another order, over the sound's spectrum,
// in and out. The panel serves in full too.
func colourView(p *colourPlugin, s plug.Size, h plug.Host) gunim.Node {
	if s == plug.Chip {
		return &colourChip{p: p, v: valuesOf(&ColourDescriptor, h), units: p.Units()}
	}
	return newChainPanel(p, h)
}

// kindColor is a kind of unit's colour.
func kindColor(th *theme.Live, kind string) color.NRGBA {
	switch kind {
	case Chorus:
		return FxChorus.Get(th)
	case Phaser:
		return FxPhaser.Get(th)
	default:
		return FxDistortion.Get(th)
	}
}

// unitTitle is what a unit's card and chip call it: its mode or type
// where it has one.
func unitTitle(u Unit, v values) string {
	switch u.Kind {
	case Chorus:
		return ChorusModeNames[stepOf(v[UnitParamID(u.Slot, ChorusModeParam)], len(ChorusModeNames))]
	case Distortion:
		return DistortTypeNames[stepOf(v[UnitParamID(u.Slot, DistortTypeParam)], len(DistortTypeNames))]
	}
	return kindName[u.Kind]
}

// The tips of the units' selectors' options.
var (
	chorusTips = []string{"Two copies, one each side, 9 ms late, moving slowly",
		"A Juno-60's chorus I: a slow sweep, the right side against the left",
		"A Juno-60's chorus II: the same sweep, faster",
		"Both of a Juno-60's buttons down: a fast, shallow vibrato",
		"A string machine's ensemble: three copies, each swept by a slow and a fast sine"}
	distortTips = []string{"A fuzz's hard clip", "A guitar amplifier's valves and its cabinet",
		"A wavefolder, which folds the sound back on itself"}
	autoGainTips = []string{"The distortion as loud as it comes out, louder the harder it drives",
		"The distortion kept as loud as the sound fed to it, however hard it drives"}
)

// unitCard is a unit's card in the chain: its section of controls, a
// button that removes it, and a header to drag it by. Its place glides
// as the chain changes, and it grows in as it is added.
type unitCard struct {
	unit    Unit
	section *plugui.Section
	sel     *plugui.Selector
	// auto switches a distortion's Auto gain.
	auto   *plugui.Selector
	remove *widget.IconButton
	// x and y are where it stands, gliding; appear grows from 0 to 1 as
	// it comes, and lift as it is dragged.
	x, y, appear, lift *anim.Float
	anim.Group
	// to is where the chain puts it, and at where it was placed last.
	to, at geom.Point
	panel  *chainPanel
	size   geom.Size
}

func newUnitCard(pn *chainPanel, u Unit) *unitCard {
	c := &unitCard{unit: u, panel: pn, x: anim.NewFloat(0), y: anim.NewFloat(0), appear: anim.NewFloat(0),
		lift: anim.NewFloat(0)}
	c.Add(c.x, c.y, c.appear, c.lift)
	h, v := pn.h, pn.v
	knob := func(place uint32, tip string) *plugui.Knob {
		prm := unitParam(u.Kind, place)
		prm.ID = UnitParamID(u.Slot, place)
		k := plugui.NewKnob(h, prm)
		k.Tooltip = tip
		return k
	}
	pick := func(place uint32) func(int, *gunim.UI) {
		id := UnitParamID(u.Slot, place)
		return func(i int, ui *gunim.UI) {
			gesture(h, id, float64(i), ui)
			v[id] = float64(i)
		}
	}
	c.section = &plugui.Section{Name: fmt.Sprintf("%d · %s", 0, strings.ToUpper(kindName[u.Kind]))}
	switch u.Kind {
	case Chorus:
		c.sel = plugui.NewSelector("Mode", ChorusModeNames, chorusTips)
		c.sel.Columns = 3
		c.sel.OnPick = pick(ChorusModeParam)
		c.sel.SetSelected(stepOf(v[UnitParamID(u.Slot, ChorusModeParam)], len(ChorusModeNames)), nil)
		c.section.Knobs = []*plugui.Knob{knob(ChorusMixParam, "How much of the chorus is heard")}
		c.section.Add(c.sel, 64)
	case Phaser:
		c.section.Knobs = []*plugui.Knob{
			knob(PhaserRateParam, "How fast the notches sweep"),
			knob(PhaserDepthParam, "How far the notches sweep"),
			knob(PhaserFeedbackParam, "How much goes round again, which sharpens the notches"),
			knob(PhaserMixParam, "How much of the phaser is heard: all of it makes the deepest notches"),
		}
	default:
		c.sel = plugui.NewSelector("Type", DistortTypeNames, distortTips)
		c.sel.OnPick = pick(DistortTypeParam)
		c.sel.SetSelected(stepOf(v[UnitParamID(u.Slot, DistortTypeParam)], len(DistortTypeNames)), nil)
		c.auto = plugui.NewSelector("Auto gain", []string{"Off", "On"}, autoGainTips)
		c.auto.OnPick = pick(DistortAutoParam)
		c.auto.SetSelected(stepOf(v[UnitParamID(u.Slot, DistortAutoParam)], 2), nil)
		c.section.Knobs = []*plugui.Knob{knob(DistortDriveParam, "How hard the sound is driven"),
			knob(DistortMixParam, "How much of the distortion is heard"),
			knob(DistortOutputParam, "How far the sound is turned up or down, after the mix")}
		c.section.Add(c.sel, 56).Add(c.auto, 56)
	}
	c.remove = widget.NewIconButton(icon.X, "Remove the "+strings.ToLower(kindName[u.Kind]))
	c.remove.OnClick = func(ui *gunim.UI) gunim.Intent {
		pn.removeUnit(c, ui)
		return nil
	}
	return c
}

// unitParam returns kind's parameter at place, as a slot's.
func unitParam(kind string, place uint32) plug.Param {
	for _, p := range unitParams[kind] {
		if p.ID == place {
			return p
		}
	}
	return plug.Param{}
}

// width is how wide the card stands.
func (c *unitCard) width() float32 { return c.section.Width() }

// follow takes a parameter set elsewhere, where it is the card's.
func (c *unitCard) follow(id uint32, v float64, u *gunim.UI) {
	c.section.Follow(id, v, u)
	if c.sel != nil && (id == UnitParamID(c.unit.Slot, ChorusModeParam) || id == UnitParamID(c.unit.Slot, DistortTypeParam)) {
		c.sel.SetSelected(int(math.Round(v)), u)
	}
	if c.auto != nil && id == UnitParamID(c.unit.Slot, DistortAutoParam) {
		c.auto.SetSelected(int(math.Round(v)), u)
	}
}

// Children implements [gunim.Composite].
func (c *unitCard) Children() []gunim.Node { return []gunim.Node{c.section, c.remove} }

// removeSize is how big the remove button is.
const removeSize = 22

// Layout implements [gunim.Node].
func (c *unitCard) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	c.size = geom.Sz(c.width(), plugui.SectionH)
	kids.At(0).Layout(gunim.Tight(c.size))
	kids.At(0).Place(geom.Point{})
	kids.At(1).Layout(gunim.Tight(geom.Sz(removeSize, removeSize)))
	kids.At(1).Place(geom.Pt(c.size.W-removeSize-3, 2))
	return c.size
}

// Paint implements [gunim.Node]: the card grows in as it comes, and
// lifts, with a shadow, as it is dragged; a stripe of its kind's colour
// runs along its top.
func (c *unitCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	a := min(max(c.appear.Value(), 0), 1)
	lift := c.lift.Value()
	if lift > 0.01 {
		p.ShadowRRect(geom.Rect{Max: box.Point()}, widget.CardRadius.Get(th), paint.Solid(audioui.Faded(widget.Ink.Get(th), 0)),
			paint.Shadow{Blur: 18 * lift, Offset: geom.Pt(0, 6*lift), Color: color.NRGBA{A: uint8(120 * lift)}})
	}
	ctr := geom.Pt(box.W/2, box.H/2)
	s := 0.92 + 0.08*a + 0.03*lift
	end := p.Push(paint.Scale(s, ctr))
	defer end()
	layer := p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: a})
	defer layer()
	for k := range kids.All {
		k.Paint(p)
	}
	col := kindColor(th, c.unit.Kind)
	p.RRect(geom.Rc(plugui.SectionPad, 3, box.W-2*plugui.SectionPad-removeSize, 2), 1, paint.Solid(audioui.Faded(col, 0.8)))
}

// Transition implements [gunim.Transitioner]: the card grows in as it
// is added, and shrinks away as it is removed.
func (c *unitCard) Transition(p gunim.Presence, _ gunim.Frame) bool {
	to := float32(1)
	if p == gunim.Exiting {
		to = 0
	}
	if c.appear.Target() != to {
		c.appear.Animate(to, anim.Spring{Response: 0.3, Damping: 0.85})
	}
	return !c.appear.Active()
}

// Handle implements [gunim.Handler]: a press on the header, beside the
// remove button, picks the card up to drag.
func (c *unitCard) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || e.Pos.Y > plugui.SectionHead {
			return false
		}
		c.panel.pickUp(c, c.at.Add(e.Pos), u)
	case input.PointerMove:
		if c.panel.dragged != c {
			return false
		}
		c.panel.dragTo(c.at.Add(e.Pos), u)
	case input.PointerUp:
		if c.panel.dragged != c {
			return false
		}
		c.panel.drop(u)
	default:
		return false
	}
	return true
}

// Overhear implements [gunim.Overhearer]: a second button pressed while
// the card is dragged drops it where it got to.
func (c *unitCard) Overhear(e input.Event, u *gunim.UI) {
	if _, ok := e.(input.PointerDown); ok && c.panel.dragged == c {
		c.panel.drop(u)
	}
}

// Access implements [gunim.Accessible].
func (c *unitCard) Access() access.Info {
	return access.Info{Role: access.RoleGroup, Name: fmt.Sprintf("%s, unit %d of the chain", c.section.Name,
		c.panel.indexOf(c)+1)}
}

// addCard adds a unit to the chain: a button for each kind. It fades
// away while the chain is full.
type addCard struct {
	buttons      []*widget.Button
	x, y, appear *anim.Float
	anim.Group
	to geom.Point
}

func newAddCard(pn *chainPanel) *addCard {
	a := &addCard{x: anim.NewFloat(0), y: anim.NewFloat(0), appear: anim.NewFloat(1)}
	a.Add(a.x, a.y, a.appear)
	for _, kind := range UnitKinds {
		b := widget.NewButton(kindName[kind])
		b.Icon, b.Ghost = icon.Plus, true
		b.Tooltip = "Add a " + strings.ToLower(kindName[kind]) + " at the end of the chain"
		b.OnClick = func(u *gunim.UI) gunim.Intent {
			pn.addUnit(kind, u)
			return nil
		}
		a.buttons = append(a.buttons, b)
	}
	return a
}

// addW is how wide the add card stands.
const addW = 150

// Children implements [gunim.Composite].
func (a *addCard) Children() []gunim.Node {
	out := make([]gunim.Node, len(a.buttons))
	for i, b := range a.buttons {
		out[i] = b
	}
	return out
}

// Layout implements [gunim.Node].
func (a *addCard) Layout(_ gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := geom.Sz(addW, plugui.SectionH)
	h := (size.H - plugui.SectionHead - plugui.SectionPad) / float32(len(a.buttons))
	for i := range a.buttons {
		kids.At(i).Layout(gunim.Tight(geom.Sz(addW-2*plugui.SectionPad, h-4)))
		kids.At(i).Place(geom.Pt(plugui.SectionPad, plugui.SectionHead+float32(i)*h))
	}
	return size
}

// Transition implements [gunim.Transitioner].
func (a *addCard) Transition(p gunim.Presence, _ gunim.Frame) bool {
	to := float32(1)
	if p == gunim.Exiting {
		to = 0
	}
	if a.appear.Target() != to {
		a.appear.Animate(to, anim.Spring{Response: 0.25, Damping: 1})
	}
	return !a.appear.Active()
}

// Paint implements [gunim.Node].
func (a *addCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(max(a.appear.Value(), 0), 1)})()
	plugui.PaintGlass(p, th, geom.Rect{Max: box.Point()}, widget.CardRadius.Get(th))
	name := audioui.Shaped("ADD", 10, true, false)
	name.Paint(p, geom.Pt(plugui.SectionPad, (plugui.SectionHead-name.Height())/2+2), audioui.Faded(widget.Ink.Get(th), 0.55))
	for k := range kids.All {
		k.Paint(p)
	}
}

// chainPanel is the chain on cards along the panel's foot, in rows as
// many as they take, an arrow from each to the next, and the add card
// after them; over the sound's spectrum, in and out, filling the panel.
type chainPanel struct {
	p     *colourPlugin
	h     plug.Host
	v     values
	units []Unit
	cards []*unitCard
	add   *addCard
	spec  *spectrum
	out   *meter
	box   *meterBox
	tuner *plugui.Tuner
	// dragged is the card being dragged, grabbed at grab from its
	// corner, and dropAt the place in the chain it would drop at now.
	dragged *unitCard
	grab    geom.Point
	dropAt  int
	size    geom.Size
	// top is where the cards' first row starts.
	top float32
}

func newChainPanel(p *colourPlugin, h plug.Host) *chainPanel {
	_, out := h.Sound()
	pn := &chainPanel{p: p, h: h, v: valuesOf(&ColourDescriptor, h), spec: newSpectrum(h), out: newMeter(out),
		tuner: plugui.NewTuner("Tune how the sound shows behind the controls")}
	pn.box = &meterBox{m: pn.out}
	pn.add = newAddCard(pn)
	pn.showUnits(p.Units(), true, nil)
	return pn
}

// showUnits shows the chain units, its cards kept for the units it had,
// made for the units new, which grow in unless first is set.
func (pn *chainPanel) showUnits(units []Unit, first bool, u *gunim.UI) {
	old := pn.cards
	pn.units, pn.cards = units, nil
	for i, unit := range units {
		var c *unitCard
		for k, o := range old {
			if o != nil && o.unit == unit {
				c, old[k] = o, nil
			}
		}
		if c == nil {
			c = newUnitCard(pn, unit)
			if first {
				c.appear.Jump(1)
			}
			if u != nil {
				u.InsertAt(pn, 2, c)
			}
		}
		c.section.Name = fmt.Sprintf("%d · %s", i+1, strings.ToUpper(kindName[unit.Kind]))
		pn.cards = append(pn.cards, c)
	}
	if u != nil {
		for _, o := range old {
			if o != nil {
				u.Remove(o)
			}
		}
		full := len(pn.cards) == MaxUnits
		switch in := u.Presence(pn.add) != gunim.Exiting; {
		case full && in:
			u.Remove(pn.add)
		case !full && !in:
			u.InsertAt(pn, 2, pn.add)
		}
	}
	pn.place(first || u == nil)
	if u != nil {
		u.Invalidate()
	}
}

// indexOf returns where card c is in the chain, or -1.
func (pn *chainPanel) indexOf(c *unitCard) int {
	for i, o := range pn.cards {
		if o == c {
			return i
		}
	}
	return -1
}

// edit is one gesture named name, making units the chain.
func (pn *chainPanel) edit(name string, units []Unit, u *gunim.UI) {
	pn.h.BeginEdit(name, u)
	pn.p.setUnits(units)
	pn.h.Changed(u)
	pn.h.EndEdit(u)
	pn.showUnits(pn.p.Units(), false, u)
}

// addUnit adds a unit of kind at the end of the chain.
func (pn *chainPanel) addUnit(kind string, u *gunim.UI) {
	if units, ok := withUnit(pn.p.Units(), kind); ok {
		pn.edit("Add "+strings.ToLower(kindName[kind]), units, u)
	}
}

// removeUnit takes card c's unit out of the chain.
func (pn *chainPanel) removeUnit(c *unitCard, u *gunim.UI) {
	if i := pn.indexOf(c); i >= 0 {
		if pn.dragged == c {
			pn.dragged = nil
		}
		pn.edit("Remove "+strings.ToLower(kindName[c.unit.Kind]), withoutUnit(pn.units, i), u)
	}
}

// pickUp picks card c up to drag, held at pos in the panel.
func (pn *chainPanel) pickUp(c *unitCard, pos geom.Point, u *gunim.UI) {
	pn.dragged, pn.grab, pn.dropAt = c, pos.Sub(c.at), pn.indexOf(c)
	c.lift.Animate(1, widget.Quick.Get(u.Theme()))
	u.Invalidate()
}

// dragTo moves the card dragged to pos, and the others aside to where
// it would drop.
func (pn *chainPanel) dragTo(pos geom.Point, u *gunim.UI) {
	c := pn.dragged
	at := pos.Sub(pn.grab)
	c.x.Jump(at.X)
	c.y.Jump(at.Y)
	// The place whose slot's middle is nearest the card's.
	mid := at.Add(geom.Pt(c.width()/2, plugui.SectionH/2))
	best, most := pn.dropAt, float32(math.Inf(1))
	me := pn.indexOf(c)
	for i := range pn.cards {
		s := pn.spots(i)[me]
		d := float32(math.Hypot(float64(mid.X-s.Min.X-c.width()/2), float64(mid.Y-s.Min.Y-plugui.SectionH/2)))
		if d < most {
			best, most = i, d
		}
	}
	if best != pn.dropAt {
		pn.dropAt = best
		pn.place(false)
	}
	u.Invalidate()
}

// drop lets the card dragged go, at the place it would drop at.
func (pn *chainPanel) drop(u *gunim.UI) {
	c := pn.dragged
	pn.dragged = nil
	c.lift.Animate(0, widget.Quick.Get(u.Theme()))
	from := pn.indexOf(c)
	if from >= 0 && pn.dropAt != from {
		pn.edit("Move "+strings.ToLower(kindName[c.unit.Kind]), moved(pn.units, from, pn.dropAt), u)
		return
	}
	pn.place(false)
	u.Invalidate()
}

// order is the cards in the order they stand, the card dragged at the
// place it would drop at.
func (pn *chainPanel) order() []*unitCard {
	if pn.dragged == nil {
		return pn.cards
	}
	i := pn.indexOf(pn.dragged)
	out := append([]*unitCard{}, pn.cards[:i]...)
	out = append(out, pn.cards[i+1:]...)
	j := min(pn.dropAt, len(out))
	return append(out[:j], append([]*unitCard{pn.dragged}, out[j:]...)...)
}

// cardGap is the room between two cards, which an arrow stands in.
const cardGap = 22

// spots returns where each card stands, with the card dragged at place
// at, in rows as many as fit, and where the add card stands after them.
func (pn *chainPanel) spots(at int) []geom.Rect {
	order := pn.cards
	if pn.dragged != nil {
		save := pn.dropAt
		pn.dropAt = at
		order = pn.order()
		pn.dropAt = save
	}
	widths := make([]float32, 0, len(order)+1)
	for _, c := range order {
		widths = append(widths, c.width())
	}
	if len(order) < MaxUnits {
		widths = append(widths, addW)
	}
	room := pn.size.W - 2*panelInset
	var rows [][]int
	row := make([]int, 0, len(widths))
	w := float32(0)
	for i, cw := range widths {
		if len(row) > 0 && w+cardGap+cw > room {
			rows, row, w = append(rows, row), nil, 0
		}
		if len(row) > 0 {
			w += cardGap
		}
		row, w = append(row, i), w+cw
	}
	rows = append(rows, row)
	out := make([]geom.Rect, len(widths))
	pn.top = pn.size.H - panelInset - float32(len(rows))*(plugui.SectionH+plugui.SectionGap) + plugui.SectionGap
	y := pn.top
	for _, r := range rows {
		rw := float32(0)
		for k, i := range r {
			if k > 0 {
				rw += cardGap
			}
			rw += widths[i]
		}
		x := (pn.size.W - rw) / 2
		for _, i := range r {
			out[i] = geom.Rc(x, y, widths[i], plugui.SectionH)
			x += widths[i] + cardGap
		}
		y += plugui.SectionH + plugui.SectionGap
	}
	// By the cards' own order, as spots answers for place at.
	if pn.dragged != nil {
		byCard := make([]geom.Rect, len(widths))
		for k, c := range order {
			byCard[pn.indexOf(c)] = out[k]
		}
		if len(widths) > len(order) {
			byCard[len(order)] = out[len(order)]
		}
		return byCard
	}
	return out
}

// place sends each card to its spot, gliding, or at once where jump is
// set; the card dragged stays with the pointer.
func (pn *chainPanel) place(jump bool) {
	if pn.size.W == 0 {
		return
	}
	spots := pn.spots(pn.dropAt)
	sp := anim.Spring{Response: 0.3, Damping: 0.85}
	for i, c := range pn.cards {
		c.to = spots[i].Min
		if c == pn.dragged {
			continue
		}
		if jump || c.x.Value() == 0 && c.y.Value() == 0 {
			c.x.Jump(c.to.X)
			c.y.Jump(c.to.Y)
			continue
		}
		c.x.Animate(c.to.X, sp)
		c.y.Animate(c.to.Y, sp)
	}
	if len(spots) > len(pn.cards) {
		pn.add.to = spots[len(pn.cards)].Min
		if jump || pn.add.x.Value() == 0 {
			pn.add.x.Jump(pn.add.to.X)
			pn.add.y.Jump(pn.add.to.Y)
		} else {
			pn.add.x.Animate(pn.add.to.X, sp)
			pn.add.y.Animate(pn.add.to.Y, sp)
		}
	}
	pn.spec.foot = pn.top - plugui.SectionGap
}

// Children implements [gunim.Composite]: the spectrum, the meter, the
// cards, the add card where the chain has room, and the tuner. Cards
// added later go in, and cards removed leave, as the chain changes.
func (pn *chainPanel) Children() []gunim.Node {
	kids := []gunim.Node{pn.spec, pn.box}
	for _, c := range pn.cards {
		kids = append(kids, c)
	}
	if len(pn.cards) < MaxUnits {
		kids = append(kids, pn.add)
	}
	return append(kids, pn.tuner.Button, pn.tuner.Card)
}

// Layout implements [gunim.Node]: each child by what it is; a card,
// and a card leaving, where it glides.
func (pn *chainPanel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	resized := pn.size != c.Max
	pn.size = c.Max
	if resized {
		pn.place(true)
	}
	for k := range kids.All {
		switch n := k.Node().(type) {
		case *spectrum:
			k.Layout(gunim.Tight(pn.size))
			k.Place(geom.Point{})
		case *meterBox:
			k.Layout(gunim.Tight(geom.Sz(meterW, 8)))
			k.Place(geom.Pt(pn.size.W-panelInset-meterW-40, panelInset+4))
		case *unitCard:
			k.Layout(gunim.Tight(geom.Sz(n.width(), plugui.SectionH)))
			n.at = geom.Pt(n.x.Value(), n.y.Value())
			k.Place(n.at)
		case *addCard:
			k.Layout(gunim.Tight(geom.Sz(addW, plugui.SectionH)))
			k.Place(geom.Pt(n.x.Value(), n.y.Value()))
		case *plugui.TunerCard:
			cs := k.Layout(gunim.Constraints{Max: geom.Sz(plugui.TunerW, plugui.TunerH)})
			k.Place(geom.Pt(pn.size.W-panelInset/2-max(cs.W, plugui.TunerW), panelInset/2+36))
		default:
			k.Layout(gunim.Tight(geom.Sz(32, 32)))
			k.Place(geom.Pt(pn.size.W-32-panelInset/2, panelInset/2))
		}
	}
	return pn.size
}

// Paint implements [gunim.Node]: the spectrum, then an arrow in each
// gap from a unit to the next, then the cards.
func (pn *chainPanel) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	ink := widget.Ink.Get(f.Theme)
	order := pn.order()
	spots := pn.spots(pn.dropAt)
	for i := 0; i+1 < len(order)+1 && i < len(order); i++ {
		a := spots[pn.indexOf(order[i])]
		var b geom.Rect
		switch {
		case i+1 < len(order):
			b = spots[pn.indexOf(order[i+1])]
		case len(spots) > len(order):
			b = spots[len(order)]
		default:
			continue
		}
		if b.Min.Y != a.Min.Y {
			continue
		}
		y := a.Min.Y + plugui.SectionHead + plugui.KnobH/2
		x := a.Max.X + cardGap/2
		audioui.Segment(p, geom.Pt(x-5, y-5), geom.Pt(x+1, y), 1.5, audioui.Faded(ink, 0.4))
		audioui.Segment(p, geom.Pt(x+1, y), geom.Pt(x-5, y+5), 1.5, audioui.Faded(ink, 0.4))
	}
	// The card dragged over the others.
	var dragged gunim.Child
	held := false
	for k := range kids.All {
		switch n := k.Node().(type) {
		case *spectrum:
		case *unitCard:
			if n == pn.dragged {
				dragged, held = k, true
				continue
			}
			k.Paint(p)
		default:
			k.Paint(p)
		}
	}
	if held {
		dragged.Paint(p)
	}
}

// Follow implements [plug.Follower].
func (pn *chainPanel) Follow(id uint32, v float64, u *gunim.UI) {
	pn.v[id] = v
	for _, c := range pn.cards {
		c.follow(id, v, u)
	}
	u.Invalidate()
}

// FollowState implements [plug.StateFollower]: the cards follow the
// chain as it is set, gliding to their new places.
func (pn *chainPanel) FollowState(u *gunim.UI) {
	if pn.dragged != nil {
		pn.dragged = nil
	}
	pn.showUnits(pn.p.Units(), false, u)
}

// Access implements [gunim.Accessible].
func (pn *chainPanel) Access() access.Info {
	names := make([]string, len(pn.units))
	for i, unit := range pn.units {
		names[i] = unitTitle(unit, pn.v) + " " + strings.ToLower(kindName[unit.Kind])
	}
	return access.Info{Role: access.RoleGroup, Name: "Chain", Value: strings.Join(names, ", then ")}
}

// The spectrum's frequencies, from viewLow to viewHigh across; its
// points; how many frames it measures; and its levels at its top and
// foot.
const (
	viewLow, viewHigh = 20.0, 20000.0
	specPoints        = 160
	specFrames        = 4096
	specTop           = 0
	specBottom        = -84
)

// spectrum draws the sound by pitch, going in, faint, and coming out,
// over it.
type spectrum struct {
	in, out plug.Sound
	frames  []float32
	meter   *audioui.Spectrometer
	freqs   []float32
	// is and was are the levels measured, out and in, and isS and wasS
	// as drawn, easing to them.
	is, was, isS, wasS []float32
	at                 int64
	still              time.Duration
	foot               float32
	size               geom.Size
}

func newSpectrum(h plug.Host) *spectrum {
	in, out := h.Sound()
	s := &spectrum{in: in, out: out, frames: make([]float32, 2*specFrames), meter: audioui.NewSpectrometer(specFrames),
		still: plug.StillAfter}
	s.freqs = make([]float32, specPoints)
	for i := range s.freqs {
		s.freqs[i] = float32(viewLow * math.Pow(viewHigh/viewLow, float64(i)/(specPoints-1)))
	}
	for _, p := range []*[]float32{&s.is, &s.was, &s.isS, &s.wasS} {
		*p = make([]float32, specPoints)
		for i := range *p {
			(*p)[i] = specBottom - 20
		}
	}
	return s
}

// Step implements [gunim.Animator]: the spectra measured as the sound
// brings more, falling once it stops.
func (s *spectrum) Step(dt time.Duration) bool {
	playing := false
	if s.out != nil {
		n, at, rate := s.out.Heard(s.frames)
		switch {
		case at != 0 && at != s.at && n == specFrames:
			s.at, s.still, playing = at, 0, true
			s.meter.Measure(s.frames, rate, s.freqs, s.is)
			if s.in != nil {
				if n, _, rate := s.in.Heard(s.frames); n == specFrames {
					s.meter.Measure(s.frames, rate, s.freqs, s.was)
				}
			}
		case at != 0:
			s.still = min(s.still+dt, plug.StillAfter)
			playing = s.still < plug.StillAfter
		}
	}
	if !playing {
		for i := range s.is {
			s.is[i], s.was[i] = specBottom-20, specBottom-20
		}
	}
	sec := float32(dt.Seconds())
	settled := true
	ease := func(v *float32, to float32) {
		rate := float32(30)
		if to < *v {
			rate = 6
		}
		*v += (to - *v) * min(1, rate*sec)
		if math.Abs(float64(to-*v)) > 0.3 {
			settled = false
		}
	}
	for i := range s.is {
		ease(&s.isS[i], s.is[i])
		ease(&s.wasS[i], s.was[i])
	}
	return playing || !settled
}

// Layout implements [gunim.Node].
func (s *spectrum) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	s.size = c.Max
	if s.foot == 0 {
		s.foot = s.size.H
	}
	return c.Max
}

func (s *spectrum) xOf(hz float64) float32 {
	return s.size.W * float32(math.Log(hz/viewLow)/math.Log(viewHigh/viewLow))
}

func (s *spectrum) yOf(db float32) float32 {
	top := float32(panelInset + 34)
	t := (db - specBottom) / (specTop - specBottom)
	return top + (s.foot-top)*(1-max(0, min(t, 1)))
}

// Paint implements [gunim.Node].
func (s *spectrum) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, accent := widget.Ink.Get(th), widget.Accent.Get(th)
	defer paintGround(p, th, box)()
	for _, hz := range []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000} {
		x := s.xOf(hz)
		p.RRect(geom.Rc(x, panelInset+34, 1, s.foot-panelInset-34), 0, paint.Solid(audioui.Faded(ink, 0.06)))
		label := audioui.Shaped(shortHz(hz), 9, false, true)
		label.Paint(p, geom.Pt(x+4, panelInset+36), audioui.Faded(ink, 0.35))
	}
	n := len(s.freqs)
	step := math.Pow(viewHigh/viewLow, 0.5/float64(n-1))
	for i, fz := range s.freqs {
		x0, x1 := s.xOf(float64(fz)/step), s.xOf(float64(fz)*step)
		if y := s.yOf(s.isS[i]); y < s.foot {
			p.RRect(geom.Rc(x0, y, x1-x0+0.5, s.foot-y), 0, paint.Solid(audioui.Faded(accent, 0.16)))
		}
	}
	plugui.Darken(p, geom.Rect{Max: box.Point()}, 1)
	lines := p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: plugui.Lines(1)})
	defer lines()
	var prevI, prevO geom.Point
	for i, fz := range s.freqs {
		x := s.xOf(float64(fz))
		in, o := geom.Pt(x, s.yOf(s.wasS[i])), geom.Pt(x, s.yOf(s.isS[i]))
		if i > 0 {
			audioui.Segment(p, prevI, in, 1, audioui.Faded(ink, 0.35))
			audioui.Segment(p, prevO, o, 1.5, audioui.Faded(accent, 0.8))
		}
		prevI, prevO = in, o
	}
	title := audioui.Shaped("Colour", 13, true, false)
	title.Paint(p, geom.Pt(panelInset+8, panelInset+4), audioui.Faded(ink, 0.8))
	legend := audioui.Shaped("in  ·  out", 10, false, false)
	legend.Paint(p, geom.Pt(panelInset+8+title.Advance+14, panelInset+7), audioui.Faded(ink, 0.45))
}

// Access implements [gunim.Accessible].
func (s *spectrum) Access() access.Info {
	return access.Info{Role: access.RoleImage, Name: "Spectrum in and out"}
}

// shortHz writes a frequency on the grid, short.
func shortHz(hz float64) string {
	if hz >= 1000 {
		return fmt.Sprintf("%.0fk", hz/1000)
	}
	return fmt.Sprintf("%.0f", hz)
}

// colourChip shows, in a plugin's chip in a chain, the chain's units, a
// pill each in its kind's colour.
type colourChip struct {
	p     *colourPlugin
	v     values
	units []Unit
}

// Layout implements [gunim.Node].
func (c *colourChip) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return cs.Constrain(geom.Sz(cs.Max.W, 20))
}

// Paint implements [gunim.Node].
func (c *colourChip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink := widget.Ink.Get(th)
	if len(c.units) == 0 {
		words := audioui.Shaped("Empty chain", plugui.ChipTextSize, false, false)
		words.Paint(p, geom.Pt(0, (box.H-words.Height())/2), audioui.Faded(ink, 0.4))
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Clip: true, Opacity: 1})()
	x := float32(0)
	h := min(box.H-4, 16)
	for i, u := range c.units {
		col := kindColor(th, u.Kind)
		words := audioui.Shaped(unitTitle(u, c.v), 10, true, false)
		w := words.Advance + 10
		p.RRect(geom.Rc(x, (box.H-h)/2, w, h), h/2, paint.Solid(audioui.Faded(col, 0.22)))
		words.Paint(p, geom.Pt(x+5, (box.H-words.Height())/2), col)
		x += w
		if i+1 < len(c.units) {
			arrow := audioui.Shaped("›", 11, true, false)
			arrow.Paint(p, geom.Pt(x+3, (box.H-arrow.Height())/2), audioui.Faded(ink, 0.45))
			x += arrow.Advance + 6
		}
	}
}

// Access implements [gunim.Accessible].
func (c *colourChip) Access() access.Info {
	names := make([]string, len(c.units))
	for i, u := range c.units {
		names[i] = unitTitle(u, c.v)
	}
	return access.Info{Role: access.RoleLabel, Name: "Colour: " + strings.Join(names, ", ")}
}

// Follow implements [plug.Follower].
func (c *colourChip) Follow(id uint32, v float64, u *gunim.UI) {
	c.v[id] = v
	u.Invalidate()
}

// FollowState implements [plug.StateFollower].
func (c *colourChip) FollowState(u *gunim.UI) {
	c.units = c.p.Units()
	u.Invalidate()
}
