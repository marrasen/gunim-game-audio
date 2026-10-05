# gunim-music

A song for [gunim](https://github.com/marrasen/gunim)'s band player, to
try a program's sound with. Ten synths made in Reason at 136 BPM: nine
come and go in 16-bar phrases, each with an intro, a loop and an
outro, and a brass solo plays now and then. The band chooses who plays
at each phrase, so the song changes as it goes, and seldom plays the
same way twice. gunim's candy sudoku plays it.

Listen to it:

```sh
go run github.com/marrasen/gunim-music/cmd/play@latest
```

Play it in a program, under its other sounds:

```go
mix := audio.NewMixer()
if _, err := speaker.Open(mix, speaker.Options{Name: "My game"}); err != nil {
	return err
}
mix.Play(music.New(seed), audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
```

`music.Song` returns the song for `band.New`, with options of your own.

## The parts

`song/` holds each synth's parts in Ogg Vorbis at 48 kHz, about 5 MB in
all: the intro is bars 1 to 16, the loop bars 17 to 32 and the outro
bars 33 to 36. The brass's 20 bars are a solo, kept whole. `encode.sh`
cuts them from the bounces, one WAV a synth, with ffmpeg:

```sh
./encode.sh ~/bounce_music
```

A song of your own plays the same way: cut its parts at its bars,
named as `name-intro.ogg`, `name-loop.ogg` and `name-outro.ogg`, and
read them with `band.Load`.

## Licence

The song, in `song/`, is © 2026 Marcus Johansson, under the
[Creative Commons Attribution 4.0](LICENSE-music) licence: use it,
change it and share it, in anything, and credit Marcus Johansson.

The Go code is under the [Apache License 2.0](LICENSE), as gunim is.
