package xair

import (
	"fmt"
	"strconv"
)

// Config is the global /config subtree. Mirrors config.py's Config.
type Config struct {
	node
	MuteGroup [4]MuteGroup
	Monitor   Monitor

	chLinkMax  int
	busLinkMax int
}

func newGlobalConfig(c *Client, k kind) *Config {
	n := node{c: c, addr: "/config"}
	cfg := &Config{node: n, Monitor: newMonitor(n), chLinkMax: k.numStrip, busLinkMax: k.numBus}
	for i := range cfg.MuteGroup {
		cfg.MuteGroup[i] = newMuteGroup(n, i)
	}
	return cfg
}

// AmixEnable reports whether the automix engine is enabled.
//
// xair-api-python's Config.amixenable getter reads "/config/mute" (a
// copy-paste bug — see WIRE_PROTOCOL.md §9); this reads the address its
// own setter actually writes, "/config/amixenable".
func (c *Config) AmixEnable() (bool, error)  { return c.getBool("amixenable") }
func (c *Config) SetAmixEnable(v bool) error { return c.setBool("amixenable", v) }

func (c *Config) AmixLock() (bool, error)  { return c.getBool("amixlock") }
func (c *Config) SetAmixLock(v bool) error { return c.setBool("amixlock", v) }

func (c *Config) LinkEQ() (bool, error)  { return c.getBool("linkcfg/eq") }
func (c *Config) SetLinkEQ(v bool) error { return c.setBool("linkcfg/eq", v) }

func (c *Config) LinkDyn() (bool, error)  { return c.getBool("linkcfg/dyn") }
func (c *Config) SetLinkDyn(v bool) error { return c.setBool("linkcfg/dyn", v) }

func (c *Config) LinkFaderMute() (bool, error)  { return c.getBool("linkcfg/fdrmute") }
func (c *Config) SetLinkFaderMute(v bool) error { return c.setBool("linkcfg/fdrmute", v) }

// ChannelLink reports the stereo link state for input strips pairStart
// and pairStart+1. pairStart must be an odd channel number (1, 3, 5, ...).
func (c *Config) ChannelLink(pairStart int) (bool, error) {
	if err := c.checkLinkPair(pairStart, c.chLinkMax); err != nil {
		return false, err
	}
	return c.getBool(fmt.Sprintf("chlink/%d-%d", pairStart, pairStart+1))
}

// SetChannelLink sets the stereo link state for input strips pairStart
// and pairStart+1.
func (c *Config) SetChannelLink(pairStart int, v bool) error {
	if err := c.checkLinkPair(pairStart, c.chLinkMax); err != nil {
		return err
	}
	return c.setBool(fmt.Sprintf("chlink/%d-%d", pairStart, pairStart+1), v)
}

// BusLink reports the stereo link state for buses pairStart and
// pairStart+1. pairStart must be an odd bus number (1, 3, 5, ...).
func (c *Config) BusLink(pairStart int) (bool, error) {
	if err := c.checkLinkPair(pairStart, c.busLinkMax); err != nil {
		return false, err
	}
	return c.getBool(fmt.Sprintf("buslink/%d-%d", pairStart, pairStart+1))
}

// SetBusLink sets the stereo link state for buses pairStart and
// pairStart+1.
func (c *Config) SetBusLink(pairStart int, v bool) error {
	if err := c.checkLinkPair(pairStart, c.busLinkMax); err != nil {
		return err
	}
	return c.setBool(fmt.Sprintf("buslink/%d-%d", pairStart, pairStart+1), v)
}

func (c *Config) checkLinkPair(pairStart, max int) error {
	if pairStart < 1 || pairStart%2 == 0 || pairStart >= max {
		return fmt.Errorf("xair: link pair must be an odd number in 1..%d, got %d", max-1, pairStart)
	}
	return nil
}

// MuteGroup is one of the mixer's 4 global mute groups
// (/config/mute/{1..4}). Mirrors config.py's Config.MuteGroup.
type MuteGroup struct {
	node
	i int
}

func newMuteGroup(parent node, i int) MuteGroup {
	return MuteGroup{node: node{c: parent.c, addr: parent.addr + "/mute"}, i: i + 1}
}

func (m MuteGroup) On() (bool, error)  { return m.getBool(strconv.Itoa(m.i)) }
func (m MuteGroup) SetOn(v bool) error { return m.setBool(strconv.Itoa(m.i), v) }

// Monitor is the /config/solo subtree. Mirrors config.py's Config.Monitor.
type Monitor struct{ node }

func newMonitor(parent node) Monitor { return Monitor{parent.child("/solo")} }

func (m Monitor) Level() (float64, error)   { return m.getDB("level") }
func (m Monitor) SetLevel(db float64) error { return m.setDB("level", db) }

func (m Monitor) Source() (int32, error)  { return m.getInt("source") }
func (m Monitor) SetSource(v int32) error { return m.setInt("source", v) }

func (m Monitor) SourceTrim() (float64, error) { return m.getLin("sourcetrim", -18, 18) }
func (m Monitor) SetSourceTrim(db float64) error {
	return m.setLin("sourcetrim", "sourcetrim", -18, 18, db)
}

func (m Monitor) ChMode() (bool, error)  { return m.getBool("chmode") }
func (m Monitor) SetChMode(v bool) error { return m.setBool("chmode", v) }

func (m Monitor) BusMode() (bool, error)  { return m.getBool("busmode") }
func (m Monitor) SetBusMode(v bool) error { return m.setBool("busmode", v) }

func (m Monitor) DimGain() (float64, error) { return m.getLin("dimatt", -40, 0) }
func (m Monitor) SetDimGain(db float64) error {
	return m.setLin("dimatt", "dimgain", -40, 0, db)
}

func (m Monitor) Dim() (bool, error)  { return m.getBool("dim") }
func (m Monitor) SetDim(v bool) error { return m.setBool("dim", v) }

func (m Monitor) Mono() (bool, error)  { return m.getBool("mono") }
func (m Monitor) SetMono(v bool) error { return m.setBool("mono", v) }

func (m Monitor) Mute() (bool, error)  { return m.getBool("mute") }
func (m Monitor) SetMute(v bool) error { return m.setBool("mute", v) }

func (m Monitor) DimFPL() (bool, error)  { return m.getBool("dimfpl") }
func (m Monitor) SetDimFPL(v bool) error { return m.setBool("dimfpl", v) }
