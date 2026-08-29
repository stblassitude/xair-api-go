package xair

import "fmt"

// node is embedded by every addressable OSC leaf/subtree wrapper. It
// pairs the shared Client with this node's own OSC address prefix,
// playing the role of xair-api-python's IStrip/IBus/... getter+setter
// mixins.
type node struct {
	c    *Client
	addr string
}

// Address returns this node's full OSC address prefix.
func (n node) Address() string { return n.addr }

func (n node) child(suffix string) node { return node{c: n.c, addr: n.addr + suffix} }

func (n node) getRaw(param string) ([]interface{}, string, error) {
	addr := n.addr + "/" + param
	args, err := n.c.GetTimeout(addr)
	return args, addr, err
}

func (n node) getFloat(param string) (float32, error) {
	args, addr, err := n.getRaw(param)
	if err != nil {
		return 0, err
	}
	return argFloat32(args, addr)
}

func (n node) getInt(param string) (int32, error) {
	args, addr, err := n.getRaw(param)
	if err != nil {
		return 0, err
	}
	return argInt32(args, addr)
}

func (n node) getString(param string) (string, error) {
	args, addr, err := n.getRaw(param)
	if err != nil {
		return "", err
	}
	return argString(args, addr)
}

func (n node) getBool(param string) (bool, error) {
	v, err := n.getInt(param)
	if err != nil {
		return false, err
	}
	return v == 1, nil
}

func (n node) setFloat(param string, v float32) error { return n.c.Send(n.addr+"/"+param, v) }
func (n node) setInt(param string, v int32) error     { return n.c.Send(n.addr+"/"+param, v) }
func (n node) setString(param string, v string) error { return n.c.Send(n.addr+"/"+param, v) }

func (n node) setBool(param string, v bool) error {
	var i int32
	if v {
		i = 1
	}
	return n.setInt(param, i)
}

// warnRange logs when a value falls outside a parameter's documented
// range but does not reject it, mirroring xair-api-python's setters:
// they warn and still send the (out-of-curve) value rather than erroring.
func (n node) warnRange(field string, val, min, max float64) {
	if val < min || val > max {
		n.c.logger.Warn(fmt.Sprintf("xair: %s got %v, expected value in range %v to %v", field, val, min, max))
	}
}

func (n node) getLin(param string, min, max float64) (float64, error) {
	v, err := n.getFloat(param)
	if err != nil {
		return 0, err
	}
	return round1(linGet(min, max, float64(v))), nil
}

func (n node) setLin(param, field string, min, max, val float64) error {
	n.warnRange(field, val, min, max)
	return n.setFloat(param, float32(linSet(min, max, val)))
}

func (n node) getLog(param string, min, max float64) (float64, error) {
	v, err := n.getFloat(param)
	if err != nil {
		return 0, err
	}
	return round1(logGet(min, max, float64(v))), nil
}

func (n node) setLog(param, field string, min, max, val float64) error {
	n.warnRange(field, val, min, max)
	return n.setFloat(param, float32(logSet(min, max, val)))
}

func (n node) getDB(param string) (float64, error) {
	v, err := n.getFloat(param)
	if err != nil {
		return 0, err
	}
	return dBGet(float64(v)), nil
}

func (n node) setDB(param string, db float64) error {
	return n.setFloat(param, float32(dBSet(db)))
}
