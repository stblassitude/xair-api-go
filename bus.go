package xair

// Bus is an output bus. Its address differs by mixer family — "/bus/N"
// (X-Air/MR, no padding) vs "/bus/NN" (X32, zero-padded) — so callers get
// it via Mixer.Bus, built with the right address per WIRE_PROTOCOL.md §8.
// Mirrors bus.py's Bus.
type Bus struct {
	node
	Config ChannelConfig
	Dyn    Dyn
	Insert Insert
	GEQ    GEQ
	EQ     EQ6
	Mix    Mix
	Group  Group
}

func newBus(c *Client, addr string) *Bus {
	n := node{c: c, addr: addr}
	return &Bus{
		node:   n,
		Config: newChannelConfig(n),
		Dyn:    newDyn(n),
		Insert: newInsert(n),
		GEQ:    newGEQ(n),
		EQ:     newEQ6(n),
		Mix:    newMix(n),
		Group:  newGroup(n),
	}
}

func (b *Bus) Mute() (bool, error) {
	on, err := b.Mix.On()
	return !on, err
}

func (b *Bus) SetMute(v bool) error { return b.Mix.SetOn(!v) }
