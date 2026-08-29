package xair

// LR is the six-band-EQ/GEQ main-mix node type shared by the X-Air/MR
// "/lr" main stereo bus and the X32's "/main/st", "/main/m", and
// "/mtx/NN" matrix buses — they all expose the identical parameter set
// and differ only in address (WIRE_PROTOCOL.md §8). Mirrors lr.py's LR
// and adapter.py's MainStereo/MainMono/Matrix.
type LR struct {
	node
	Config ChannelConfig
	Dyn    Dyn
	Insert Insert
	GEQ    GEQ
	EQ     EQ6
	Mix    Mix
}

func newLR(c *Client, addr string) *LR {
	n := node{c: c, addr: addr}
	return &LR{
		node:   n,
		Config: newChannelConfig(n),
		Dyn:    newDyn(n),
		Insert: newInsert(n),
		GEQ:    newGEQ(n),
		EQ:     newEQ6(n),
		Mix:    newMix(n),
	}
}

func (l *LR) Mute() (bool, error) {
	on, err := l.Mix.On()
	return !on, err
}

func (l *LR) SetMute(v bool) error { return l.Mix.SetOn(!v) }

// Matrix (X32 only) has the same parameter set as LR/Main; only the
// address differs.
type Matrix = LR
