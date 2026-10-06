package calls

import (
	"encoding/binary"
	"io"
	"math"
)

// WriteWAV writes samples to w as a WAV file, mono, 24 bits at the
// mixer's rate.
func WriteWAV(w io.Writer, samples []float32) error {
	data := 3 * len(samples)
	h := make([]byte, 44, 44+data)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+data))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(3*rate))
	binary.LittleEndian.PutUint16(h[32:], 3)
	binary.LittleEndian.PutUint16(h[34:], 24)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(data))
	for _, v := range samples {
		s := int32(math.Round(float64(min(max(v, -1), 1)) * (1<<23 - 1)))
		h = append(h, byte(s), byte(s>>8), byte(s>>16))
	}
	_, err := w.Write(h)
	return err
}
