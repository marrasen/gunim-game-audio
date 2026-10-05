#!/bin/sh
# encode.sh makes a song's parts from its bounces: one WAV an
# instrument, all starting at bar 1, from a folder given, into the
# song's folder, whose song.json gives the tempo and the bars.
#
#	./encode.sh ~/bounce_music songs/greek-themes
#
# A bounce longer than two phrases is cut at its bars into three parts:
# the intro, the first phrase; the loop, the second; and the outro, the
# rest. A shorter bounce, or one whose name ends in "solo", is a solo,
# kept whole. Each part is Ogg Vorbis at 48 kHz, the rate gunim mixes
# at. It takes ffmpeg, with libvorbis and soxr, and jq.
set -eu
src=${1:?usage: encode.sh folder-of-wavs song-folder}
out=${2:?usage: encode.sh folder-of-wavs song-folder}
bpm=$(jq -r .BPM "$out/song.json")
beats=$(jq -r .BeatsPerBar "$out/song.json")
phrase=$(jq -r .PhraseBars "$out/song.json")

# at prints the frame bar $1 starts at, at 48 kHz, rounded as gunim's
# band counts it, so every part starts where the band does.
at() { awk -v b="$1" -v bpm="$bpm" -v n="$beats" 'BEGIN { printf "%d", b * 48000 * 60 * n / bpm + 0.5 }'; }

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
	name=$(basename "$f" .wav | sed 's/^[0-9]*\. *//' | tr 'A-Z ' 'a-z-')
	frames=$(ffprobe -v error -select_streams a:0 -show_entries stream=duration_ts -of csv=p=0 "$f")
	rate=$(ffprobe -v error -select_streams a:0 -show_entries stream=sample_rate -of csv=p=0 "$f")
	bars=$(awk -v f="$frames" -v r="$rate" -v bpm="$bpm" -v n="$beats" 'BEGIN { printf "%d", f / r * bpm / 60 / n + 0.5 }')
	case $name in
	*-solo) part "$f" "$name" 0 ""; echo "$name: $bars bars, a solo"; continue ;;
	esac
	if [ "$bars" -gt $((2 * phrase)) ]; then
		part "$f" "$name-intro" 0 "$(at "$phrase")"
		part "$f" "$name-loop" "$(at "$phrase")" "$(at $((2 * phrase)))"
		part "$f" "$name-outro" "$(at $((2 * phrase)))" ""
		echo "$name: $bars bars"
	else
		part "$f" "$name-solo" 0 ""
		echo "$name: $bars bars, a solo"
	fi
done
