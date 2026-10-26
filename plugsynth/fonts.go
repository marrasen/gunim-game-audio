package plugsynth

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"weak"

	"github.com/marrasen/gunim-game-audio/soundfont"
	"github.com/marrasen/gunim-game-audio/synth"
)

// Recorded is the bank, in the browser, of the instruments of the banks
// of one instrument each, each filed under its Group, as "Piano".
const Recorded = "Recorded instruments"

// A Font is a SoundFont bank whose instruments the synth offers as
// sounds, sampled, once a program has read it and handed it to UseFont.
type Font struct {
	// Name is the name its sounds play it by, as synth.RegisterSoundFont
	// registers it: a song or a state names the bank by it, so it stays
	// as it is from version to version, as a soundfont.Bank's Key does.
	Name string
	// Title names it, as "MuseScore General" or "Upright Piano KW".
	Title string
	// Group files a bank of one instrument under Recorded, as "Piano".
	// "" makes it a General MIDI bank, a bank of the browser's of its
	// own, by Title, its instruments filed by General MIDI's families
	// and its kits under GMDrums.
	Group string
	// Kit says a bank of one instrument plays a drum kit, or
	// percussion, a sound to a key.
	Kit bool
	// Level is how many decibels its sounds are turned by, for it to
	// play about as loud as the others.
	Level float64
	// About says what it is, and Credit who made it: a sound of a bank
	// of one instrument says both.
	About, Credit string
	// Font is the bank, read.
	Font *soundfont.Font
}

// fonts are the banks UseFont was handed, by Name, with their sounds,
// guarded by fontsMu; sounds are every sound, the module's and the
// banks', as fonts change.
var (
	fontsMu sync.Mutex
	fonts   = map[string]fontSounds{}
	sounds  atomic.Pointer[[]Sound]
)

// fontSounds is a bank and its sounds.
type fontSounds struct {
	Font
	sounds []Sound
}

// UseFont registers bank f as synth.RegisterSoundFont does, by its Name,
// and offers its instruments among the sounds, in place of a bank of
// the same Name before. Each synth playing a sound of the bank, which
// played the engine's own instruments while the bank was missing, plays
// the bank from then on.
func UseFont(f Font) {
	if f.Font == nil {
		DropFont(f.Name)
		return
	}
	fs := fontSounds{Font: f}
	if f.Group == "" {
		fs.sounds = gmFontSounds(f)
	} else {
		fs.sounds = oneFontSounds(f)
	}
	synth.RegisterSoundFont(f.Name, f.Font)
	fontsMu.Lock()
	fonts[f.Name] = fs
	fontsMu.Unlock()
	resound()
	replay(f.Name)
}

// DropFont lets the bank of name go: its sounds leave the list, and the
// synths playing one play the engine's own instruments in its place.
func DropFont(name string) {
	synth.RegisterSoundFont(name, nil)
	fontsMu.Lock()
	_, had := fonts[name]
	delete(fonts, name)
	fontsMu.Unlock()
	if had {
		resound()
		replay(name)
	}
}

// Fonts returns the banks UseFont was handed, by Title.
func Fonts() []Font {
	fontsMu.Lock()
	out := make([]Font, 0, len(fonts))
	for _, f := range fonts {
		out = append(out, f.Font)
	}
	fontsMu.Unlock()
	slices.SortFunc(out, func(a, b Font) int { return cmp.Compare(a.Title, b.Title) })
	return out
}

// fontNamed returns the bank UseFont was handed by name, and whether
// there is one.
func fontNamed(name string) (Font, bool) {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	f, ok := fonts[name]
	return f.Font, ok
}

// all returns every sound: the module's own, and those of the banks.
func all() []Sound {
	if p := sounds.Load(); p != nil {
		return *p
	}
	resound()
	return *sounds.Load()
}

// resound lists the sounds afresh, the module's and the banks' as they
// stand: General MIDI's banks after General MIDI, by Title, then the
// recorded instruments, by group and Title, then the songs'.
func resound() {
	own := ownSounds()
	// The list is made and stored under the lock, so two banks handed
	// over at once leave the list holding both.
	fontsMu.Lock()
	defer fontsMu.Unlock()
	var gm, one []fontSounds
	for _, f := range fonts {
		if f.Group == "" {
			gm = append(gm, f)
		} else {
			one = append(one, f)
		}
	}
	slices.SortFunc(gm, func(a, b fontSounds) int { return cmp.Compare(a.Title, b.Title) })
	slices.SortFunc(one, func(a, b fontSounds) int {
		return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Title, b.Title))
	})
	out := make([]Sound, 0, len(own)+256)
	at := 0
	for at < len(own) && own[at].Bank != Songs {
		at++
	}
	out = append(out, own[:at]...)
	for _, f := range gm {
		out = append(out, f.sounds...)
	}
	for _, f := range one {
		out = append(out, f.sounds...)
	}
	out = append(out, own[at:]...)
	sounds.Store(&out)
}

// fontPatch is the patch that plays program prog of bank of f.
func fontPatch(f Font, bank, prog int, kit bool) *synth.Patch {
	p := &synth.Patch{SoundFont: f.Name, Bank: bank, Program: prog, FontKit: kit && bank != synth.DrumBank, Poly: 32}
	if f.Level != 0 {
		p.Gain = math.Pow(10, f.Level/20)
	}
	return p
}

// gmFontSounds are the sounds of General MIDI bank f: its instruments
// by General MIDI's families, the variations of each beside it, and its
// kits.
func gmFontSounds(f Font) []Sound {
	ps := slices.Clone(f.Font.Presets)
	slices.SortStableFunc(ps, func(a, b soundfont.Preset) int {
		ak, bk := a.Bank == synth.DrumBank, b.Bank == synth.DrumBank
		switch {
		case ak != bk && ak:
			return 1
		case ak != bk:
			return -1
		}
		return cmp.Or(cmp.Compare(a.Program, b.Program), cmp.Compare(a.Bank, b.Bank))
	})
	var out []Sound
	taken := map[[2]string]bool{}
	for _, p := range ps {
		if p.Program < 0 || p.Program > 127 {
			continue
		}
		kit := p.Bank == synth.DrumBank
		group := GMDrums
		about := fmt.Sprintf("%s's drum kit %d, recorded.", f.Title, p.Program+1)
		if !kit {
			group = gmFamilies[p.Program/8]
			about = fmt.Sprintf("General MIDI's program %d, recorded, from %s.", p.Program+1, f.Title)
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = synth.GMNames[p.Program]
		}
		for n := 2; taken[[2]string{group, name}]; n++ {
			name = fmt.Sprintf("%s %d", strings.TrimSpace(p.Name), n)
		}
		taken[[2]string{group, name}] = true
		s := Sound{Bank: f.Title, Group: group, Name: name, About: about, patch: fontPatch(f, p.Bank, p.Program, kit)}
		s.drums, s.pitch = kit, keyOf(f.Font, p, kit)
		out = append(out, s)
	}
	return out
}

// oneFontSounds are the sounds of f, a bank of one instrument, under
// Recorded: its first preset by its Title, the others by their names.
func oneFontSounds(f Font) []Sound {
	about := strings.TrimSpace(f.About + " " + f.Credit)
	out := make([]Sound, 0, len(f.Font.Presets))
	for i, p := range f.Font.Presets {
		name := f.Title
		if i > 0 {
			name = strings.TrimSpace(p.Name)
		}
		s := Sound{Bank: Recorded, Group: f.Group, Name: name, About: about, patch: fontPatch(f, p.Bank, p.Program, f.Kit)}
		s.drums, s.pitch = f.Kit || p.Bank == synth.DrumBank, keyOf(f.Font, p, f.Kit)
		out = append(out, s)
	}
	return out
}

// keyOf is the key a preview of preset p of f plays: a kick, 36, for a
// kit that has one, or else its lowest key; middle C for an instrument
// that plays it, or else the middle of what it plays.
func keyOf(f *soundfont.Font, p soundfont.Preset, kit bool) float64 {
	lo, hi := -1, -1
	for k := range 128 {
		if len(f.Regions(p.Bank, p.Program, k, 100)) > 0 {
			if lo < 0 {
				lo = k
			}
			hi = k
		}
	}
	switch {
	case lo < 0:
		if kit {
			return 36
		}
		return 60
	case kit && lo <= 36 && 36 <= hi:
		return 36
	case kit:
		return float64(lo)
	case lo <= 60 && 60 <= hi:
		return 60
	}
	return float64((lo + hi) / 2)
}

// synths are the synths open, for UseFont and DropFont to have those
// playing a bank play it afresh. It holds them weakly: a synth let go
// without Close leaves it as the collector takes it.
var synths sync.Map // weak.Pointer[plugin] → struct{}

// replay has each synth playing a sound of the bank of name play it
// afresh, on the bank as it is registered now.
func replay(name string) {
	synths.Range(func(k, _ any) bool {
		wp, _ := k.(weak.Pointer[plugin])
		p := wp.Value()
		if p == nil {
			return true
		}
		st := p.sound()
		if st.Patch == nil || st.Patch.SoundFont != name {
			return true
		}
		if snd, err := newSound(st.Patch); err == nil {
			p.proc.offer(snd)
		}
		return true
	})
}
