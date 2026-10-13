package plugfx

import "github.com/marrasen/gunim/audio/plug"

func init() {
	plug.Register(ReverbDescriptor)
	plug.Register(DelayDescriptor)
	plug.Register(ColourDescriptor)
}
