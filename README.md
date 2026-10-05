# gunim-music

A library of songs for [gunim](https://github.com/marrasen/gunim)'s band
player, package `audio/band`, to try a program's sound with, as a
game's music. Each song plays without end, and changes as it goes.

| Song | Name | What it is |
|---|---|---|
| Greek Themes | `greek-themes` | Ten synths made in Reason at 136 BPM: nine come and go in 16-bar phrases, and a brass solo plays now and then. gunim's candy sudoku plays it. |

Listen to one:

```sh
go run github.com/marrasen/gunim-music/cmd/play@latest -list
go run github.com/marrasen/gunim-music/cmd/play@latest -song greek-themes
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

Each seed plays a song its own way. Some songs will offer more than
playing, as tiers that grow with a game's combo, or moods such as
underwater. Each such feature is an interface of package `band` that a
song's player implements, so a program checks for it:

```go
p := song.Play(seed)
if t, ok := p.(band.Tiered); ok { // once a song has tiers
	t.SetTier(tier)
}
```

## Adding a song

Each song is a folder in `songs/`, named for the song, with its parts
and a `song.json`. Its `Kind` says what plays it: `wander` is a
`band.Wander`, whose parts come and go in phrases.

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

A wander's parts are Ogg Vorbis at 48 kHz, named for the instrument and
the piece: `blade-intro.ogg`, `blade-loop.ogg` and `blade-outro.ogg`,
or `greek-power-brass-solo.ogg` for a solo. The intro is the first
phrase, the loop the second, and the outro the rest. `encode.sh` cuts
them from the bounces, one WAV an instrument from bar 1, with ffmpeg,
reading the tempo and the phrase from the folder's `song.json`:

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
