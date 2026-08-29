package xair

import "fmt"

// FX is an effects engine slot (/fx/N). Mirrors fx.py's FX.
type FX struct{ node }

func newFX(c *Client, index int) *FX {
	return &FX{node{c: c, addr: fmt.Sprintf("/fx/%d", index+1)}}
}

func (f *FX) Type() (int32, error)  { return f.getInt("type") }
func (f *FX) SetType(v int32) error { return f.setInt("type", v) }

// FXSend is the pre-FX send bus feeding an FX slot (/fxsend/N, X-Air/MR
// family only — the X32 has no equivalent node). Mirrors fx.py's FXSend.
type FXSend struct {
	node
	Config ChannelConfig
	Mix    Mix
	Group  Group
}

func newFXSend(c *Client, index int) *FXSend {
	n := node{c: c, addr: fmt.Sprintf("/fxsend/%d", index+1)}
	return &FXSend{node: n, Config: newChannelConfig(n), Mix: newMix(n), Group: newGroup(n)}
}

func (f *FXSend) Mute() (bool, error) {
	on, err := f.Mix.On()
	return !on, err
}

func (f *FXSend) SetMute(v bool) error { return f.Mix.SetOn(!v) }
