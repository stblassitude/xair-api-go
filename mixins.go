package xair

import "fmt"

// ChannelConfig is the /config subtree shared by every channel-like node
// (strip, bus, fx send/return, aux return, lr/main/matrix). Mirrors
// shared.py's Config.
type ChannelConfig struct{ node }

func newChannelConfig(parent node) ChannelConfig { return ChannelConfig{parent.child("/config")} }

func (c ChannelConfig) Name() (string, error)  { return c.getString("name") }
func (c ChannelConfig) SetName(v string) error { return c.setString("name", v) }
func (c ChannelConfig) Color() (int32, error)  { return c.getInt("color") }
func (c ChannelConfig) SetColor(v int32) error { return c.setInt("color", v) }

func (c ChannelConfig) InputSource() (int32, error)  { return c.getInt("insrc") }
func (c ChannelConfig) SetInputSource(v int32) error { return c.setInt("insrc", v) }

func (c ChannelConfig) USBReturn() (int32, error)  { return c.getInt("rtnsrc") }
func (c ChannelConfig) SetUSBReturn(v int32) error { return c.setInt("rtnsrc", v) }

// Preamp is the /preamp subtree (strip, fx return, aux return). Mirrors
// shared.py's Preamp.
type Preamp struct{ node }

func newPreamp(parent node) Preamp { return Preamp{parent.child("/preamp")} }

func (p Preamp) USBTrim() (float64, error)   { return p.getLin("rtntrim", -18, 18) }
func (p Preamp) SetUSBTrim(db float64) error { return p.setLin("rtntrim", "usbtrim", -18, 18, db) }

func (p Preamp) USBInput() (bool, error)  { return p.getBool("rtnsw") }
func (p Preamp) SetUSBInput(v bool) error { return p.setBool("rtnsw", v) }

func (p Preamp) Invert() (bool, error)  { return p.getBool("invert") }
func (p Preamp) SetInvert(v bool) error { return p.setBool("invert", v) }

func (p Preamp) HighPassOn() (bool, error)  { return p.getBool("hpon") }
func (p Preamp) SetHighPassOn(v bool) error { return p.setBool("hpon", v) }

func (p Preamp) HighPassFilter() (float64, error) { return p.getLog("hpf", 20, 400) }
func (p Preamp) SetHighPassFilter(hz float64) error {
	return p.setLog("hpf", "highpassfilter", 20, 400, hz)
}

// Gate is the /gate subtree (strip only). Mirrors shared.py's Gate.
type Gate struct{ node }

func newGate(parent node) Gate { return Gate{parent.child("/gate")} }

var gateModes = []string{"exp2", "exp3", "exp4", "gate", "duck"}

func (g Gate) On() (bool, error)  { return g.getBool("on") }
func (g Gate) SetOn(v bool) error { return g.setBool("on", v) }

func (g Gate) Mode() (string, error) {
	i, err := g.getInt("mode")
	if err != nil {
		return "", err
	}
	return enumName(gateModes, i)
}

func (g Gate) SetMode(v string) error {
	i, err := enumIndex(gateModes, v)
	if err != nil {
		return err
	}
	return g.setInt("mode", int32(i))
}

func (g Gate) Threshold() (float64, error)   { return g.getLin("thr", -80, 0) }
func (g Gate) SetThreshold(db float64) error { return g.setLin("thr", "threshold", -80, 0, db) }

func (g Gate) Range() (float64, error)  { return g.getLin("range", 3, 60) }
func (g Gate) SetRange(v float64) error { return g.setLin("range", "range", 3, 60, v) }

func (g Gate) Attack() (float64, error)   { return g.getLin("attack", 0, 120) }
func (g Gate) SetAttack(ms float64) error { return g.setLin("attack", "attack", 0, 120, ms) }

func (g Gate) Hold() (float64, error)   { return g.getLog("hold", 0.02, 2000) }
func (g Gate) SetHold(ms float64) error { return g.setLog("hold", "hold", 0.02, 2000, ms) }

func (g Gate) Release() (float64, error)   { return g.getLog("release", 5, 4000) }
func (g Gate) SetRelease(ms float64) error { return g.setLog("release", "release", 5, 4000, ms) }

func (g Gate) KeySource() (int32, error)  { return g.getInt("keysrc") }
func (g Gate) SetKeySource(v int32) error { return g.setInt("keysrc", v) }

func (g Gate) FilterOn() (bool, error)  { return g.getBool("filter/on") }
func (g Gate) SetFilterOn(v bool) error { return g.setBool("filter/on", v) }

func (g Gate) FilterType() (int32, error)  { return g.getInt("filter/type") }
func (g Gate) SetFilterType(v int32) error { return g.setInt("filter/type", v) }

func (g Gate) FilterFreq() (float64, error) { return g.getLog("filter/f", 20, 20000) }
func (g Gate) SetFilterFreq(hz float64) error {
	return g.setLog("filter/f", "filterfreq", 20, 20000, hz)
}

// Dyn is the /dyn subtree (strip, bus, lr/main/matrix). Mirrors
// shared.py's Dyn.
type Dyn struct{ node }

func newDyn(parent node) Dyn { return Dyn{parent.child("/dyn")} }

var (
	dynModes  = []string{"comp", "exp"}
	dynDets   = []string{"peak", "rms"}
	dynEnvs   = []string{"lin", "log"}
	dynRatios = []float64{1.1, 1.3, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 7.0, 10, 20, 100}
)

func (d Dyn) On() (bool, error)  { return d.getBool("on") }
func (d Dyn) SetOn(v bool) error { return d.setBool("on", v) }

func (d Dyn) Mode() (string, error) {
	i, err := d.getInt("mode")
	if err != nil {
		return "", err
	}
	return enumName(dynModes, i)
}

func (d Dyn) SetMode(v string) error {
	i, err := enumIndex(dynModes, v)
	if err != nil {
		return err
	}
	return d.setInt("mode", int32(i))
}

func (d Dyn) Det() (string, error) {
	i, err := d.getInt("det")
	if err != nil {
		return "", err
	}
	return enumName(dynDets, i)
}

func (d Dyn) SetDet(v string) error {
	i, err := enumIndex(dynDets, v)
	if err != nil {
		return err
	}
	return d.setInt("det", int32(i))
}

func (d Dyn) Env() (string, error) {
	i, err := d.getInt("env")
	if err != nil {
		return "", err
	}
	return enumName(dynEnvs, i)
}

func (d Dyn) SetEnv(v string) error {
	i, err := enumIndex(dynEnvs, v)
	if err != nil {
		return err
	}
	return d.setInt("env", int32(i))
}

func (d Dyn) Threshold() (float64, error)   { return d.getLin("thr", -60, 0) }
func (d Dyn) SetThreshold(db float64) error { return d.setLin("thr", "threshold", -60, 0, db) }

// Ratio returns the compression ratio, decoded from the mixer's 12-entry
// lookup table (WIRE_PROTOCOL.md §6).
func (d Dyn) Ratio() (float64, error) {
	i, err := d.getInt("ratio")
	if err != nil {
		return 0, err
	}
	if i < 0 || int(i) >= len(dynRatios) {
		return 0, fmt.Errorf("xair: dyn: ratio index %d out of range", i)
	}
	return dynRatios[i], nil
}

// SetRatio sets the ratio by lookup-table index (0..11), matching
// xair-api-python's setter, which does not validate the index either.
func (d Dyn) SetRatio(index int32) error { return d.setInt("ratio", index) }

func (d Dyn) Knee() (float64, error)  { return d.getLin("knee", 0, 5) }
func (d Dyn) SetKnee(v float64) error { return d.setLin("knee", "knee", 0, 5, v) }

func (d Dyn) MGain() (float64, error)   { return d.getLin("mgain", 0, 24) }
func (d Dyn) SetMGain(db float64) error { return d.setLin("mgain", "mgain", 0, 24, db) }

func (d Dyn) Attack() (float64, error)   { return d.getLin("attack", 0, 120) }
func (d Dyn) SetAttack(ms float64) error { return d.setLin("attack", "attack", 0, 120, ms) }

func (d Dyn) Hold() (float64, error)   { return d.getLog("hold", 0.02, 2000) }
func (d Dyn) SetHold(ms float64) error { return d.setLog("hold", "hold", 0.02, 2000, ms) }

func (d Dyn) Release() (float64, error)   { return d.getLog("release", 5, 4000) }
func (d Dyn) SetRelease(ms float64) error { return d.setLog("release", "release", 5, 4000, ms) }

func (d Dyn) Mix() (float64, error)  { return d.getLin("mix", 0, 100) }
func (d Dyn) SetMix(v float64) error { return d.setLin("mix", "mix", 0, 100, v) }

func (d Dyn) KeySource() (int32, error)  { return d.getInt("keysrc") }
func (d Dyn) SetKeySource(v int32) error { return d.setInt("keysrc", v) }

func (d Dyn) Auto() (bool, error)  { return d.getBool("auto") }
func (d Dyn) SetAuto(v bool) error { return d.setBool("auto", v) }

func (d Dyn) FilterOn() (bool, error)  { return d.getBool("filter/on") }
func (d Dyn) SetFilterOn(v bool) error { return d.setBool("filter/on", v) }

func (d Dyn) FilterType() (int32, error)  { return d.getInt("filter/type") }
func (d Dyn) SetFilterType(v int32) error { return d.setInt("filter/type", v) }

func (d Dyn) FilterFreq() (float64, error) { return d.getLog("filter/f", 20, 20000) }
func (d Dyn) SetFilterFreq(hz float64) error {
	return d.setLog("filter/f", "filterfreq", 20, 20000, hz)
}

// Insert is the /insert subtree. Mirrors shared.py's Insert.
type Insert struct{ node }

func newInsert(parent node) Insert { return Insert{parent.child("/insert")} }

func (i Insert) On() (bool, error)  { return i.getBool("on") }
func (i Insert) SetOn(v bool) error { return i.setBool("on", v) }

func (i Insert) Sel() (int32, error)  { return i.getInt("sel") }
func (i Insert) SetSel(v int32) error { return i.setInt("sel", v) }

// EQBand is one parametric EQ band (/eq/N under the channel address).
// Mirrors shared.py's EQ.EQBand.
type EQBand struct{ node }

func newEQBand(parent node, i int) EQBand {
	return EQBand{parent.child(fmt.Sprintf("/eq/%d", i))}
}

func (b EQBand) Type() (int32, error)  { return b.getInt("type") }
func (b EQBand) SetType(v int32) error { return b.setInt("type", v) }

func (b EQBand) Frequency() (float64, error) { return b.getLog("f", 20, 20000) }
func (b EQBand) SetFrequency(hz float64) error {
	return b.setLog("f", "frequency", 20, 20000, hz)
}

func (b EQBand) Gain() (float64, error)   { return b.getLin("g", -15, 15) }
func (b EQBand) SetGain(db float64) error { return b.setLin("g", "gain", -15, 15, db) }

// Quality returns the EQ band's Q factor. The wire encoding is inverted
// (wire = 1 - log_set(...)), matching shared.py's EQBand.quality exactly.
func (b EQBand) Quality() (float64, error) {
	v, err := b.getFloat("q")
	if err != nil {
		return 0, err
	}
	return round1(logGet(0.3, 10, 1.0-float64(v))), nil
}

func (b EQBand) SetQuality(q float64) error {
	b.warnRange("quality", q, 0.3, 10)
	wire := 1.0 - logSet(0.3, 10, q)
	return b.setFloat("q", float32(wire))
}

var eqModes = []string{"peq", "geq", "teq"}

type eqBase struct{ node }

func (e eqBase) On() (bool, error)  { return e.getBool("on") }
func (e eqBase) SetOn(v bool) error { return e.setBool("on", v) }

func (e eqBase) Mode() (string, error) {
	i, err := e.getInt("mode")
	if err != nil {
		return "", err
	}
	return enumName(eqModes, i)
}

func (e eqBase) SetMode(v string) error {
	i, err := enumIndex(eqModes, v)
	if err != nil {
		return err
	}
	return e.setInt("mode", int32(i))
}

// EQ4 is the four-band parametric EQ used by strips, fx returns, and aux
// returns. Mirrors shared.py's EQ.make_fourband.
type EQ4 struct {
	eqBase
	Low, LoMid, HiMid, High EQBand
}

func newEQ4(parent node) EQ4 {
	return EQ4{
		eqBase: eqBase{parent.child("/eq")},
		Low:    newEQBand(parent, 1),
		LoMid:  newEQBand(parent, 2),
		HiMid:  newEQBand(parent, 3),
		High:   newEQBand(parent, 4),
	}
}

// EQ6 is the six-band parametric EQ used by buses and lr/main/matrix.
// Mirrors shared.py's EQ.make_sixband.
type EQ6 struct {
	eqBase
	Low, Low2, LoMid, HiMid, High2, High EQBand
}

func newEQ6(parent node) EQ6 {
	return EQ6{
		eqBase: eqBase{parent.child("/eq")},
		Low:    newEQBand(parent, 1),
		Low2:   newEQBand(parent, 2),
		LoMid:  newEQBand(parent, 3),
		HiMid:  newEQBand(parent, 4),
		High2:  newEQBand(parent, 5),
		High:   newEQBand(parent, 6),
	}
}

// geqBands lists the 31 graphic-EQ band tokens, in address order. Mirrors
// shared.py's GEQ.make band list (with the '_'→'.' substitution already
// applied, since Go string constants don't need the identifier escape
// Python's attribute names required).
var geqBands = []string{
	"20", "25", "31.5", "40", "50", "63", "80", "100", "125",
	"160", "200", "250", "315", "400", "500", "630", "800", "1k",
	"1k25", "1k6", "2k", "2k5", "3k15", "4k", "5k", "6k3", "8k",
	"10k", "12k5", "16k", "20k",
}

// GEQBands returns the valid band tokens accepted by GEQ.Slider/SetSlider.
func GEQBands() []string { return append([]string(nil), geqBands...) }

// GEQ is the /geq subtree (bus and lr/main/matrix only). Mirrors
// shared.py's GEQ.
type GEQ struct{ node }

func newGEQ(parent node) GEQ { return GEQ{parent.child("/geq")} }

func (g GEQ) Slider(band string) (float64, error) {
	if _, err := enumIndex(geqBands, band); err != nil {
		return 0, fmt.Errorf("xair: geq: unknown band %q", band)
	}
	return g.getLin(band, -15, 15)
}

func (g GEQ) SetSlider(band string, db float64) error {
	if _, err := enumIndex(geqBands, band); err != nil {
		return fmt.Errorf("xair: geq: unknown band %q", band)
	}
	return g.setLin(band, "slider_"+band, -15, 15, db)
}

// Mix is the /mix subtree shared by every channel-like node except DCA.
// Mirrors shared.py's Mix.
type Mix struct{ node }

func newMix(parent node) Mix { return Mix{parent.child("/mix")} }

func (m Mix) On() (bool, error)  { return m.getBool("on") }
func (m Mix) SetOn(v bool) error { return m.setBool("on", v) }

func (m Mix) Fader() (float64, error)   { return m.getDB("fader") }
func (m Mix) SetFader(db float64) error { return m.setDB("fader", db) }

func (m Mix) LR() (bool, error)  { return m.getBool("lr") }
func (m Mix) SetLR(v bool) error { return m.setBool("lr", v) }

// Group is the /grp subtree. Mirrors shared.py's Group.
type Group struct{ node }

func newGroup(parent node) Group { return Group{parent.child("/grp")} }

func (g Group) DCA() (int32, error)  { return g.getInt("dca") }
func (g Group) SetDCA(v int32) error { return g.setInt("dca", v) }

func (g Group) Mute() (int32, error)  { return g.getInt("mute") }
func (g Group) SetMute(v int32) error { return g.setInt("mute", v) }

// Automix is the /automix subtree (strip only). Mirrors shared.py's
// Automix.
type Automix struct{ node }

func newAutomix(parent node) Automix { return Automix{parent.child("/automix")} }

func (a Automix) Group() (int32, error)  { return a.getInt("group") }
func (a Automix) SetGroup(v int32) error { return a.setInt("group", v) }

func (a Automix) Weight() (float64, error)   { return a.getLin("weight", -12, 12) }
func (a Automix) SetWeight(db float64) error { return a.setLin("weight", "weight", -12, 12, db) }

// Send is a per-bus/fx mix send (.../mix/NN). Mirrors shared.py's Send.
type Send struct{ node }

func newSend(parent node, i int) Send {
	return Send{parent.child(fmt.Sprintf("/mix/%02d", i+1))}
}

func (s Send) Level() (float64, error)   { return s.getDB("level") }
func (s Send) SetLevel(db float64) error { return s.setDB("level", db) }
