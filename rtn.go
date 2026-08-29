package xair

// FxRtn is an FX return channel. Mirrors rtn.py's FxRtn.
type FxRtn struct {
	node
	Config ChannelConfig
	Preamp Preamp
	EQ     EQ4
	Mix    Mix
	Group  Group
	Send   []Send
}

func newFxRtn(c *Client, addr string, numSends int) *FxRtn {
	n := node{c: c, addr: addr}
	r := &FxRtn{
		node:   n,
		Config: newChannelConfig(n),
		Preamp: newPreamp(n),
		EQ:     newEQ4(n),
		Mix:    newMix(n),
		Group:  newGroup(n),
	}
	r.Send = make([]Send, numSends)
	for i := range r.Send {
		r.Send[i] = newSend(n, i)
	}
	return r
}

func (r *FxRtn) Mute() (bool, error) {
	on, err := r.Mix.On()
	return !on, err
}

func (r *FxRtn) SetMute(v bool) error { return r.Mix.SetOn(!v) }

// AuxRtn is the aux return channel. It has the same parameter set as
// FxRtn on this protocol — only its address(es) differ per mixer family
// (a single fixed "/rtn/aux" on X-Air/MR vs an array of "/auxin/NN" on
// X32, see WIRE_PROTOCOL.md §8) — so it is a type alias rather than a
// duplicated struct. Mirrors rtn.py's AuxRtn.
type AuxRtn = FxRtn
