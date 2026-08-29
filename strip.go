package xair

import "fmt"

// Strip is an input channel strip (/ch/NN). Mirrors strip.py's Strip.
type Strip struct {
	node
	Config  ChannelConfig
	Preamp  Preamp
	Gate    Gate
	Dyn     Dyn
	Insert  Insert
	EQ      EQ4
	Mix     Mix
	Group   Group
	Automix Automix
	Send    []Send
}

func newStrip(c *Client, index, numSends int) *Strip {
	n := node{c: c, addr: fmt.Sprintf("/ch/%02d", index+1)}
	s := &Strip{
		node:    n,
		Config:  newChannelConfig(n),
		Preamp:  newPreamp(n),
		Gate:    newGate(n),
		Dyn:     newDyn(n),
		Insert:  newInsert(n),
		EQ:      newEQ4(n),
		Mix:     newMix(n),
		Group:   newGroup(n),
		Automix: newAutomix(n),
	}
	s.Send = make([]Send, numSends)
	for i := range s.Send {
		s.Send[i] = newSend(n, i)
	}
	return s
}

// Mute reports the strip's mute state, the logical negation of Mix.On
// (WIRE_PROTOCOL.md's shared.py mute_prop pattern).
func (s *Strip) Mute() (bool, error) {
	on, err := s.Mix.On()
	return !on, err
}

// SetMute sets the strip's mute state via Mix.SetOn(!v).
func (s *Strip) SetMute(v bool) error { return s.Mix.SetOn(!v) }
