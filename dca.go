package xair

import "fmt"

// DCA is a DCA group (/dca/N). Mirrors dca.py's DCA.
type DCA struct{ node }

func newDCA(c *Client, index int) *DCA {
	return &DCA{node{c: c, addr: fmt.Sprintf("/dca/%d", index+1)}}
}

func (d *DCA) On() (bool, error)  { return d.getBool("on") }
func (d *DCA) SetOn(v bool) error { return d.setBool("on", v) }

// Mute is the logical negation of On, matching dca.py's mute property
// (DCA has no /mix subtree, so it mutes directly via /dca/N/on).
func (d *DCA) Mute() (bool, error) {
	on, err := d.On()
	return !on, err
}

func (d *DCA) SetMute(v bool) error { return d.SetOn(!v) }

func (d *DCA) Name() (string, error)  { return d.getString("config/name") }
func (d *DCA) SetName(v string) error { return d.setString("config/name", v) }

func (d *DCA) Color() (int32, error)  { return d.getInt("config/color") }
func (d *DCA) SetColor(v int32) error { return d.setInt("config/color", v) }
