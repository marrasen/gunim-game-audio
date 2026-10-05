#!/bin/sh
# encode.sh makes the band's parts from the song's bounces: one 16-bit
# WAV a synth, 36 bars at 136 BPM, from a folder given.
#
#	./encode.sh ~/bounce_music
#
# Each synth's bounce is cut at its bars into three parts, in Ogg
# Vorbis at 48 kHz, the rate gunim mixes at: the intro, bars 1 to
# 16; the loop, bars 17 to 32; and the outro, bars 33 to 36. A bounce
# of another length, as the brass's 20 bars, is a solo, kept whole.
# It takes ffmpeg, with libvorbis and soxr.
set -eu
src=${1:?usage: encode.sh folder-of-wavs}
out=$(dirname "$0")/song

# at prints the frame bar $1 starts at, at 48 kHz: 48000 * 240 / 136
# frames a bar, rounded, so every part starts where the band counts it.
at() { awk -v b="$1" 'BEGIN { printf "%d", b * 48000 * 240 / 136 + 0.5 }'; }

# part writes frames $3 to $4 of bounce $1, $4 empty for its end, to
# part file $2.
part() {
	trim="atrim=start_sample=$3"
	[ -n "$4" ] && trim="$trim:end_sample=$4"
	ffmpeg -hide_banner -loglevel error -y -i "$1" \
		-af "aresample=48000:resampler=soxr,$trim,asetpts=N/SR/TB" \
		-c:a libvorbis -q:a 1 -map_metadata -1 "$out/$2.ogg"
}

for f in "$src"/*.wav; do
	name=$(basename "$f" .wav | tr 'A-Z ' 'a-z-')
	frames=$(ffprobe -v error -select_streams a:0 -show_entries stream=duration_ts -of csv=p=0 "$f")
	rate=$(ffprobe -v error -select_streams a:0 -show_entries stream=sample_rate -of csv=p=0 "$f")
	bars=$(awk -v n="$frames" -v r="$rate" 'BEGIN { printf "%d", n / r * 136 / 240 + 0.5 }')
	if [ "$bars" -eq 36 ]; then
		part "$f" "$name-intro" 0 "$(at 16)"
		part "$f" "$name-loop" "$(at 16)" "$(at 32)"
		part "$f" "$name-outro" "$(at 32)" ""
	else
		part "$f" "$name-solo" 0 ""
	fi
	echo "$name: $bars bars"
done
