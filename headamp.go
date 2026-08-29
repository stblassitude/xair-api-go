package xair

// HeadAmp is a channel preamp (phantom power / gain). Its address differs
// by mixer family — "/headamp/NN" (X-Air/MR, one per input strip) vs
// "/headamp/NNN" (X32, up to 127) — see WIRE_PROTOCOL.md §8. Mirrors
// headamp.py's HeadAmp.
type HeadAmp struct{ node }

func newHeadAmp(c *Client, addr string) *HeadAmp { return &HeadAmp{node{c: c, addr: addr}} }

func (h *HeadAmp) Gain() (float64, error)   { return h.getLin("gain", -12, 60) }
func (h *HeadAmp) SetGain(db float64) error { return h.setLin("gain", "gain", -12, 60, db) }

func (h *HeadAmp) Phantom() (bool, error)  { return h.getBool("phantom") }
func (h *HeadAmp) SetPhantom(v bool) error { return h.setBool("phantom", v) }
