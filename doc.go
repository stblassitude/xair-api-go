// Package xair implements a Go client for the OSC-based remote control
// protocol spoken by the Behringer X-Air / Midas MR digital mixer family
// and the Behringer X32, mirroring the feature set of xair-api-python.
//
// Beyond that feature set, Client.Subscribe consumes the mixer's
// /xremote parameter-change push, so callers can react to changes made
// elsewhere instead of polling for them.
//
// See WIRE_PROTOCOL.md for the wire protocol this package implements, and
// the "xair-api-python's OSC approach vs. Go OSC libraries" section of
// README.md for the specific correctness and concurrency issues this
// transport fixes relative to the Python reference implementation.
package xair
