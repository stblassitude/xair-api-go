package xair

import "fmt"

// ConnectionTimeoutError is returned by Connect when the mixer never
// answers /xinfo within the configured connect timeout. It mirrors
// xair-api-python's XAirRemoteConnectionTimeoutError.
type ConnectionTimeoutError struct {
	IP   string
	Port int
}

func (e *ConnectionTimeoutError) Error() string {
	return fmt.Sprintf("xair: timeout attempting to connect to mixer at %s:%d", e.IP, e.Port)
}
