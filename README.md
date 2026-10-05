# gunim-music

A library of songs for [gunim](https://github.com/marrasen/gunim)'s band
player, package `audio/band`, to try a program's sound with, as a
game's music. Each song plays without end, and changes as it goes.

| Song | Name | What it is |
|---|---|---|
| Greek Themes | `greek-themes` | Ten synths made in Reason at 136 BPM: nine come and go in 16-bar phrases, and a brass solo plays now and then. gunim's candy sudoku plays it. |
| A round song | `a-round-song` | Eight parts made in Reason at 110 BPM, in four tiers that grow with a game's combo: a pad and bass always; light percussion; three melodies; drums and a solo that loops twice, leaves and comes round again. 8-bar phrases. |

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

## Licence

The songs, in `songs/`, are © 2026 Marcus Johansson, under the
[Creative Commons Attribution 4.0](LICENSE-music) licence: use them,
change them and share them, in anything, and credit Marcus Johansson.

The Go code is under the [Apache License 2.0](LICENSE), as gunim is.
