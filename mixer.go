package xair

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// KindID identifies a supported mixer model. Mirrors kinds.py's KindMap
// id_ values.
type KindID string

const (
	KindX32  KindID = "X32"
	KindMR18 KindID = "MR18"
	KindXR18 KindID = "XR18"
	KindXR16 KindID = "XR16"
	KindXR12 KindID = "XR12"
)

type kind struct {
	id         KindID
	numDCA     int
	numStrip   int
	numBus     int
	numFX      int
	numAuxRtn  int
	numMatrix  int
	numHeadAmp int
}

// kindTable mirrors kinds.py's per-model channel/bus/etc. counts.
var kindTable = map[KindID]kind{
	KindX32:  {id: KindX32, numDCA: 8, numStrip: 32, numBus: 16, numFX: 8, numAuxRtn: 8, numMatrix: 6, numHeadAmp: 127},
	KindMR18: {id: KindMR18, numDCA: 4, numStrip: 16, numBus: 6, numFX: 4, numAuxRtn: 1},
	KindXR18: {id: KindXR18, numDCA: 4, numStrip: 16, numBus: 6, numFX: 4, numAuxRtn: 1},
	KindXR16: {id: KindXR16, numDCA: 4, numStrip: 16, numBus: 4, numFX: 4, numAuxRtn: 1},
	KindXR12: {id: KindXR12, numDCA: 4, numStrip: 12, numBus: 2, numFX: 4, numAuxRtn: 1},
}

func (k kind) defaultPort() int {
	if k.id == KindX32 {
		return 10023
	}
	return 10024
}

// Mixer is a connected mixer of a specific KindID. Only the fields
// relevant to that kind's family are populated: LR and FXSend for the
// X-Air/MR family; MainStereo, MainMono, and Matrix for X32
// (WIRE_PROTOCOL.md §8).
//
// The embedded Client also promotes Subscribe, for consuming the mixer's
// unsolicited parameter-change pushes instead of polling.
type Mixer struct {
	*Client
	KindID KindID

	LR         *LR
	MainStereo *LR
	MainMono   *LR
	Matrix     []*Matrix

	Strip     []*Strip
	Bus       []*Bus
	DCA       []*DCA
	FX        []*FX
	FXSend    []*FXSend
	FXReturn  []*FxRtn
	AuxReturn []*AuxRtn
	HeadAmp   []*HeadAmp
	Config    *Config
}

type connectOptions struct {
	port            int
	requestTimeout  time.Duration
	connectTimeout  time.Duration
	xremoteInterval time.Duration
	logger          *slog.Logger
}

// Option configures Connect.
type Option func(*connectOptions)

// WithPort overrides the mixer port (default 10023 for X32, 10024
// otherwise).
func WithPort(p int) Option { return func(o *connectOptions) { o.port = p } }

// WithRequestTimeout overrides how long a single Get waits for a reply
// (default DefaultRequestTimeout).
func WithRequestTimeout(d time.Duration) Option {
	return func(o *connectOptions) { o.requestTimeout = d }
}

// WithConnectTimeout overrides how long Connect retries /xinfo before
// giving up (default 2s, mirroring xair-api-python's connect_timeout).
func WithConnectTimeout(d time.Duration) Option {
	return func(o *connectOptions) { o.connectTimeout = d }
}

// WithXRemoteInterval overrides how often /xremote is re-sent while the
// mixer has at least one Subscribe caller (default
// DefaultXRemoteInterval).
func WithXRemoteInterval(d time.Duration) Option {
	return func(o *connectOptions) { o.xremoteInterval = d }
}

// WithLogger overrides the logger used for warnings (default
// slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(o *connectOptions) { o.logger = l } }

// Connect opens a UDP client for the given mixer kind at ip and validates
// connectivity by retrying /xinfo until it answers or the connect timeout
// (or ctx) expires, mirroring xair-api-python's validate_connection.
func Connect(ctx context.Context, kindID KindID, ip string, opts ...Option) (*Mixer, error) {
	k, ok := kindTable[kindID]
	if !ok {
		return nil, fmt.Errorf("xair: unknown mixer kind %q", kindID)
	}

	o := connectOptions{
		port:            k.defaultPort(),
		requestTimeout:  DefaultRequestTimeout,
		connectTimeout:  2 * time.Second,
		xremoteInterval: DefaultXRemoteInterval,
		logger:          slog.Default(),
	}
	for _, opt := range opts {
		opt(&o)
	}

	c, err := NewClient(ip, o.port,
		WithClientRequestTimeout(o.requestTimeout),
		WithClientXRemoteInterval(o.xremoteInterval),
		WithClientLogger(o.logger))
	if err != nil {
		return nil, err
	}

	m := &Mixer{Client: c, KindID: kindID}
	if kindID == KindX32 {
		buildX32(m, k)
	} else {
		buildXAir(m, k)
	}

	connectCtx, cancel := context.WithTimeout(ctx, o.connectTimeout)
	defer cancel()
	if err := m.validateConnection(connectCtx); err != nil {
		c.Close()
		return nil, err
	}
	return m, nil
}

func (m *Mixer) validateConnection(ctx context.Context) error {
	for {
		args, err := m.Client.Get(ctx, "/xinfo")
		if err == nil {
			ip, _ := argString(args, "/xinfo")
			var name string
			if len(args) > 2 {
				if s, ok := args[2].(string); ok {
					name = s
				}
			}
			m.logger.Info("xair: connected", "ip", ip, "name", name)
			return nil
		}
		select {
		case <-ctx.Done():
			return &ConnectionTimeoutError{IP: m.ip, Port: m.port}
		default:
		}
	}
}

// Send is the low-level escape hatch mirroring xair-api-python's
// mixer.send: it fire-and-forgets addr with the given already
// OSC-wire-typed argument(s) (WIRE_PROTOCOL.md §6).
func (m *Mixer) Send(addr string, args ...any) error { return m.Client.Send(addr, args...) }

// Query is the low-level escape hatch mirroring xair-api-python's
// mixer.query: it GETs addr and returns its raw reply arguments.
func (m *Mixer) Query(addr string) ([]any, error) { return m.Client.GetTimeout(addr) }

func buildXAir(m *Mixer, k kind) {
	c := m.Client
	numSends := k.numBus + k.numFX

	m.LR = newLR(c, "/lr")

	m.Strip = make([]*Strip, k.numStrip)
	for i := range m.Strip {
		m.Strip[i] = newStrip(c, i, numSends)
	}

	m.Bus = make([]*Bus, k.numBus)
	for i := range m.Bus {
		m.Bus[i] = newBus(c, fmt.Sprintf("/bus/%d", i+1))
	}

	m.DCA = make([]*DCA, k.numDCA)
	for i := range m.DCA {
		m.DCA[i] = newDCA(c, i)
	}

	m.FX = make([]*FX, k.numFX)
	m.FXSend = make([]*FXSend, k.numFX)
	m.FXReturn = make([]*FxRtn, k.numFX)
	for i := 0; i < k.numFX; i++ {
		m.FX[i] = newFX(c, i)
		m.FXSend[i] = newFXSend(c, i)
		m.FXReturn[i] = newFxRtn(c, fmt.Sprintf("/rtn/%d", i+1), numSends)
	}

	m.AuxReturn = []*AuxRtn{newFxRtn(c, "/rtn/aux", numSends)}

	m.HeadAmp = make([]*HeadAmp, k.numStrip)
	for i := range m.HeadAmp {
		m.HeadAmp[i] = newHeadAmp(c, fmt.Sprintf("/headamp/%02d", i+1))
	}

	m.Config = newGlobalConfig(c, k)
}

func buildX32(m *Mixer, k kind) {
	c := m.Client
	numSends := k.numBus + k.numFX

	m.MainStereo = newLR(c, "/main/st")
	m.MainMono = newLR(c, "/main/m")

	m.Matrix = make([]*Matrix, k.numMatrix)
	for i := range m.Matrix {
		m.Matrix[i] = newLR(c, fmt.Sprintf("/mtx/%02d", i+1))
	}

	m.Strip = make([]*Strip, k.numStrip)
	for i := range m.Strip {
		m.Strip[i] = newStrip(c, i, numSends)
	}

	m.Bus = make([]*Bus, k.numBus)
	for i := range m.Bus {
		m.Bus[i] = newBus(c, fmt.Sprintf("/bus/%02d", i+1))
	}

	m.DCA = make([]*DCA, k.numDCA)
	for i := range m.DCA {
		m.DCA[i] = newDCA(c, i)
	}

	m.FX = make([]*FX, k.numFX)
	for i := range m.FX {
		m.FX[i] = newFX(c, i)
	}

	m.FXReturn = make([]*FxRtn, k.numFX)
	for i := range m.FXReturn {
		m.FXReturn[i] = newFxRtn(c, fmt.Sprintf("/fxrtn/%02d", i+1), numSends)
	}

	m.AuxReturn = make([]*AuxRtn, k.numAuxRtn)
	for i := range m.AuxReturn {
		m.AuxReturn[i] = newFxRtn(c, fmt.Sprintf("/auxin/%02d", i+1), numSends)
	}

	m.HeadAmp = make([]*HeadAmp, k.numHeadAmp)
	for i := range m.HeadAmp {
		m.HeadAmp[i] = newHeadAmp(c, fmt.Sprintf("/headamp/%03d", i+1))
	}

	m.Config = newGlobalConfig(c, k)
}
