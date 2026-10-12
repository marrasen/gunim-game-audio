// Package sftest builds a small SoundFont bank for tests.
package sftest

import (
	"bytes"
	"encoding/binary"
)

// Build builds a bank of one preset, program 0 of bank 0, playing one
// instrument of one zone, keys 0 to 127, of one sample: a square wave of
// 100 frames at 44100 Hz, its root key 60, looped through all of it, with
// a release of about a tenth of a second and the pan hard right. A drum
// kit, program 0 of bank 128, plays the same instrument on keys 36 to 38
// only.
func Build() []byte {
	le := func(b *bytes.Buffer, v ...any) {
		for _, x := range v {
			_ = binary.Write(b, binary.LittleEndian, x)
		}
	}
	chunk := func(id string, body []byte) []byte {
		var b bytes.Buffer
		b.WriteString(id)
		le(&b, uint32(len(body)))
		b.Write(body)
		if len(body)%2 == 1 {
			b.WriteByte(0)
		}
		return b.Bytes()
	}
	list := func(kind string, items ...[]byte) []byte {
		body := []byte(kind)
		for _, it := range items {
			body = append(body, it...)
		}
		return chunk("LIST", body)
	}
	name := func(s string, n int) []byte { b := make([]byte, n); copy(b, s); return b }
	var smpl bytes.Buffer
	for i := range 100 + 46 {
		v := int16(12000)
		if i%100 >= 50 {
			v = -12000
		}
		le(&smpl, v)
	}
	var phdr, pbag, pmod, pgen, inst, ibag, imod, igen, shdr bytes.Buffer
	phdr.Write(name("Square", 20))
	le(&phdr, uint16(0), uint16(0), uint16(0), uint32(0), uint32(0), uint32(0))
	phdr.Write(name("Kit", 20))
	le(&phdr, uint16(0), uint16(128), uint16(1), uint32(0), uint32(0), uint32(0))
	phdr.Write(name("EOP", 20))
	le(&phdr, uint16(0), uint16(0), uint16(2), uint32(0), uint32(0), uint32(0))
	le(&pbag, uint16(0), uint16(0), uint16(1), uint16(0), uint16(3), uint16(0))
	pmod.Write(make([]byte, 10))
	le(&pgen, uint16(41), uint16(0), // the square's instrument
		uint16(43), uint8(36), uint8(38), uint16(41), uint16(0), // the kit's: keys 36 to 38
		uint16(0), uint16(0))
	inst.Write(name("Square", 20))
	le(&inst, uint16(0))
	inst.Write(name("EOI", 20))
	le(&inst, uint16(1))
	le(&ibag, uint16(0), uint16(0), uint16(5), uint16(0))
	imod.Write(make([]byte, 10))
	le(&igen,
		uint16(43), uint8(0), uint8(127), // keys 0 to 127
		uint16(54), uint16(1), // looped
		uint16(17), int16(500), // hard right
		uint16(38), int16(-4000), // release, about 0.1 s
		uint16(53), uint16(0), // the sample
		uint16(0), uint16(0))
	shdr.Write(name("square", 20))
	le(&shdr, uint32(0), uint32(100), uint32(0), uint32(100), uint32(44100), uint8(60), int8(0), uint16(0), uint16(1))
	shdr.Write(name("EOS", 20))
	le(&shdr, uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint8(0), int8(0), uint16(0), uint16(0))
	info := list("INFO", chunk("ifil", []byte{2, 0, 1, 0}), chunk("INAM", []byte("Test Bank\x00")))
	sdta := list("sdta", chunk("smpl", smpl.Bytes()))
	pdta := list("pdta", chunk("phdr", phdr.Bytes()), chunk("pbag", pbag.Bytes()), chunk("pmod", pmod.Bytes()),
		chunk("pgen", pgen.Bytes()), chunk("inst", inst.Bytes()), chunk("ibag", ibag.Bytes()), chunk("imod", imod.Bytes()),
		chunk("igen", igen.Bytes()), chunk("shdr", shdr.Bytes()))
	body := append([]byte("sfbk"), append(info, append(sdta, pdta...)...)...)
	return chunk("RIFF", body)
}
