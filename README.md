# gunim-music

A library of songs for [gunim](https://github.com/marrasen/gunim)'s band
player, package `audio/band`, to try a program's sound with, as a
game's music. Each song plays without end, and changes as it goes.

| Song | Name | What it is |
|---|---|---|
| Greek Themes | `greek-themes` | Ten synths made in Reason at 136 BPM: nine come and go in 16-bar phrases, and a brass solo plays now and then. gunim's candy sudoku plays it. |
| A round song | `a-round-song` | Eight parts made in Reason at 110 BPM, in four tiers that grow with a game's combo: a pad and bass always; light percussion; three melodies; drums and a solo that loops twice, leaves and comes round again. 8-bar phrases. |
| Keypad Round | `keypad-round` | A round song made in code, at 118 BPM in C major, in four tiers: a pad and bass always; claps and a plucked arpeggio; the lead's hook; full drums, a sparkle and a vocal chop. A riser marks each climb. The digit keys play the C major pentatonic over it. |
| Boss Entrance | `boss-entrance` | A cartoon villain's entrance, made in code, at 146 BPM in D minor, in four tiers that rise as the boss's health falls: pizzicato and a tuba; brass stabs and a timpani march; the villain's theme and a choir; drums and string runs. A victory sting of 6 bars ends it. |
| Mascot Dance | `mascot-dance` | A bright dance groove, made in code, at 124 BPM in E major, for a mascot to dance to. Its tracks come and go by themselves around a kick, a bass and hats that always play, and a topline it writes itself changes every two phrases. |

Try them in the jukebox, a window that plays a song, sets its tier
with buttons or the keys 1 to 4, and shows what each part plays, bar by
bar. In A round song a click on a part starts it, or stops it with its
outro, at the next phrase, and Follow the tier hands every part back to
the tier:

```sh
go run github.com/marrasen/gunim-music/cmd/jukebox@latest
```

Or listen from the terminal:

```sh
go run github.com/marrasen/gunim-music/cmd/play@latest -list
go run github.com/marrasen/gunim-music/cmd/play@latest -song greek-themes
go run github.com/marrasen/gunim-music/cmd/play@latest -song a-round-song -tier 3
```

Play one in a program, under its other sounds:

```go
mix := audio.NewMixer()
if _, err := speaker.Open(mix, speaker.Options{Name: "My game"}); err != nil {
	return err
}
song, err := music.Song(music.GreekThemes)
if err != nil {
	return err
}
mix.Play(song.Play(seed), audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
```

Each seed plays a song its own way. A song may offer more than
playing. Each such feature is an interface of package `band` that its
player implements, so a program checks for it. A song with tiers has a
`band.Tiered` player:

```go
p := song.Play(seed)
if t, ok := p.(band.Tiered); ok {
	t.SetTier(tierFor(combo)) // from the next phrase on
}
```

A tier change starts at the next phrase. The parts above the tier
leave with their outros, and those up to it come in with their intros.

A program can also start and stop the parts one by one, over the
tiers. The player is then a `band.Triggered` too:

```go
if t, ok := p.(band.Triggered); ok {
	t.SetPart("drums", band.PartOn)  // in with its intro, at the next phrase
	t.SetPart("bass", band.PartOff)  // out with its outro
	t.SetPart("bass", band.PartAuto) // back to the tier
}
```

A part keeps its control through changes of tier, until `SetPart`
changes it.

## Songs made in code

Keypad Round, Boss Entrance and Mascot Dance are made in code, by package
`synth`: synthesizers play them as they are heard, so they take edits
as they play, and a program can do more with them. Each is a
`*synth.Song`, and its player a `*synth.Player`, which is a
`band.Tiered`, a `band.Triggered` and a `band.Watcher` like the others:

```go
p := song.Play(seed).(*synth.Player)
p.SetTier(tierFor(combo)) // 1 always, 2 from a combo of 2, 3 from 4, 4 from 6
p.Key(digit)              // a note of the key's pentatonic, at once
p.Sting("victory")        // from the next beat, in place of the song
```

The tier changes at the next phrase. Where a song marks its
transitions, a riser plays in the bar before a climb, an impact lands
with it, and a down sweeps as the tier falls. A boss's health maps to
the tier the same way: full health tier 1, and each quarter lost one
tier more.

The keypad plays the pentatonic of the song's key, so a player's typing
always fits the music: in Keypad Round the keys 1 to 9 climb C, D, E,
G, A from C5, and 0 tops them. A song in another key, or transposed,
retunes the keypad with it. A sting plays from the next beat, then
silence, or the song again where the sting resumes it.

`Look` says what plays as it is heard, for visuals that keep time with
the music, such as a mascot's dance: the bar, where it started, how
long it lasts, the chord, and each track's level. Pass the frame heard,
which the voice's position gives:

```go
v := mix.Play(p, audio.Options{})
l := p.Look(audio.Frames(v.Position()))
beat := float64(audio.Frames(v.Position())-l.BarFrame) / l.BarFrames * float64(l.BeatsPerBar)
```

The engine makes a second of the busiest song, all twelve of Mascot
Dance's tracks at once, in about 35 ms on one core of a 2015 desktop
CPU, about 28 times faster than it plays.

## gunim music studio

The studio is a window to make game music in, with the engine playing
as you work:

```sh
go run github.com/marrasen/gunim-music/cmd/studio@latest
```

![gunim music studio](cmd/studio/studio.png)

The stage shows the song in 3D. Each track is an orb on the ring of its
tier, tier 1 innermost, swelling as the track grows loud. Each note
flies from its orb to the core as a spark, high or low by its pitch, so
an arpeggio climbs and falls in the air. Each drum ripples on the floor.
The chords stand round the core, the one heard raised and lit. A drag
turns the stage, and a tap on an orb starts or stops its track at the
next phrase.

Under the stage the notes flow past the line where they are heard: each
track's notes in its colour, joined so an arpeggio shows its shape, the
drums in lanes under them, and the chords above. The notes of the bar
to come show as outlines before they sound.

Each track's card takes a new pattern as it is typed, heard from the
next bar; a pattern that does not play says why, and the old one plays
on. Its sliders set its level, its filter and what it sends to the
reverb and the delay, heard at once. The chords take a new progression
as it is typed, as `Am F C G` or `vi IV I V`, or write one in a style:
pop, kpop, epic or villain. The tier, the stings, the tempo and the key
change as it plays. The digit keys play the keypad, F1 to F4 set the
tier, and Space plays and pauses. Save writes the song as a song.json.

## Adding a song

Each song is a folder in `songs/`, named for the song, with its parts
and a `song.json`. Its `Kind` says what plays it: `wander` is a
`band.Wander`, whose parts come and go in phrases, and `tiers` a
`band.Tiers`, whose parts play in tiers.

```json
{
	"Kind": "wander",
	"Title": "Greek Themes",
	"Artist": "Marcus Johansson",
	"BPM": 136,
	"BeatsPerBar": 4,
	"PhraseBars": 16
}
```

A tiers song's `song.json` also gives each part its tier, and `Loops`
for a part that loops that many times, leaves with its outro and comes
in again at the phrase after, as a solo does:

```json
	"Parts": {
		"bass": {"Tier": 1},
		"drums": {"Tier": 4},
		"solo": {"Tier": 4, "Loops": 2}
	}
```

The parts are Ogg Vorbis at 48 kHz, named for the instrument and
the piece: `blade-intro.ogg`, `blade-loop.ogg` and `blade-outro.ogg`,
or `greek-power-brass-solo.ogg` for a solo. The intro is the first
phrase, the loop the second, and the outro the rest. `encode.sh` cuts
them from the bounces, one WAV an instrument from bar 1, with ffmpeg,
reading the tempo and the phrase from the folder's `song.json`. A
number in front of a bounce's name, as in `01. Bass.wav`, is dropped:

```sh
./encode.sh ~/bounce_music songs/greek-themes
```

Then add the song's name as a constant in `music.go`, and a row above.
The tests check that every song loads, plays, and has its parts cut at
its bars.

### A song made in code

A song made in code is all in its `song.json`, of kind `synth`, with no
recordings: its key and chords, its patches, and its tracks. The studio
saves one. This is a shortened Keypad Round:

```json
{
	"Kind": "synth",
	"Title": "Keypad Round",
	"Artist": "Marcus Johansson",
	"BPM": 118,
	"Key": "C",
	"Scale": "major",
	"Chords": ["C", "Am7", "Fmaj7", "Gsus4"],
	"Mode": "tiers",
	"Keypad": {"Patch": "marimba"},
	"Patches": {
		"pad": {
			"Osc": [{"Wave": "saw", "Unison": 5, "Spread": 22}],
			"Filter": {"Type": "lp", "Cutoff": 1100, "Res": 0.12},
			"Amp": {"Attack": 0.7, "Decay": 1.2, "Sustain": 0.85, "Release": 1.4}
		},
		"kit": {"Kind": "drums"}
	},
	"Tracks": [
		{"Name": "pad", "Tier": 1, "Patch": "pad", "Pattern": "ch", "Reverb": 0.4},
		{"Name": "drums", "Tier": 4, "Patch": "kit", "Pattern": "[bd ~ ~ bd, ~ sn, hh*8]"}
	]
}
```

A track's pattern is TidalCycles' mini-notation over a bar, or over
`Bars` bars:

| Pattern | Plays |
|---|---|
| `bd ~ sn ~` | four steps, two of them rests |
| `[0 2]*2 4` | a step of two, twice, and one more |
| `<0 2 4>` | one of them each bar, in turn |
| `bd(3,8)` | three hits over eight steps, Euclid's way |
| `hh*16?` | sixteen hats, each left out half the time |
| `0 _ 2@2` | a step held for two, and one weighing two |
| `[bd, hh*4]` | both at once |

A note is a degree of the scale, as `0` for the root or `4#`; a chord
tone, as `c0`, `c1`, `c2`; `ch` for the whole chord, voiced near the
last; `b` for its bass; `x` for the next note of an arpeggio; or a note
by name, as `C4`. Each `'` after a note lifts it an octave. A drums
patch plays drums by name: `bd`, `sn`, `cp`, `hh`, `oh`, `rim`, `lt`,
`mt`, `ht`, `cr`, `rd`, `sh`, `snap`, `tim`, a timpani tuned to the
chord, and the effects `boom`, `riser` and `down`.

A track's `Params` change each note, as TidalCycles' controls do: `vel`,
`pan`, `cutoff`, `res`, `legato`, `octave`, `vowel` and `tune`, each a
pattern of values, or a signal from lo to hi as `sine:400:2000:4`. Its
mix is `Gain`, `Pan`, `Reverb`, `Delay`, a sidechain `Duck` to the
song's kick, and the effects `HPF`, `LPF`, `Shape`, `Crush`, `Coarse`
and `Chorus`.

`cmd/render` renders a song made in code to a WAV file and says how loud
each tier is, and each track alone, octave by octave, for mixing without
a speaker:

```sh
go run ./cmd/render -song boss-entrance -o boss.wav -tracks -bands
```

## Licence

The songs, in `songs/`, are © 2026 Marcus Johansson, under the
[Creative Commons Attribution 4.0](LICENSE-music) licence: use them,
change them and share them, in anything, and credit Marcus Johansson.

The Go code is under the [Apache License 2.0](LICENSE), as gunim is.
