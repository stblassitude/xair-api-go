# XAir Go -- Client to control Behringer Audio Mixers

This Go module implements a Behringer XAir-specific layer on top of
Open Sound Control, allowing you to control Behringer audio mixers. It
is based on [xair-api-python](https://github.com/onyx-and-iris/xair-api-python).

The implementation has been facilitated with the use of generative AI.

## Status

This module (package `xair`, import path `github.com/stblassitude/xair-api-go`) implements the
full feature set of `xair-api-python` — connection/kind handling, and the
Strip/Bus/DCA/FX/FXSend/FxRtn/AuxRtn/LR/Main/Matrix/HeadAmp/Config object
model with the same value-encoding conventions (see `WIRE_PROTOCOL.md`).

It builds on [`hypebeast/go-osc`](https://github.com/hypebeast/go-osc) for
OSC message encoding/decoding (`osc.Message`, `MarshalBinary`,
`ParsePacket`), but not on its `Client`/`Server`/`Dispatcher` types —
see `transport.go` and the design discussion below for why, and how the
issues identified there are fixed:

- **Address-routed replies with a timeout, not a shared "last packet"
  field plus a fixed sleep.** `Client.Get` sends a GET and waits on a
  channel registered for that exact address; `Client.dispatch` routes
  each incoming reply only to a request that's actually waiting on its
  address. See `TestClientGetRoutesRepliesByAddressConcurrently` and
  `TestClientGetTimesOutWithoutCorruptingLaterRequests` in
  `transport_test.go`, which reproduce the exact race this replaces.
- **One socket for both directions**, `net.DialUDP` "connected" to the
  mixer, matching how these mixers actually reply (to the request's
  source port) and filtering out anything not from the mixer at the OS
  level — see the `Client` doc comment in `transport.go`.
- **Concurrency-safe**: multiple goroutines can issue `Get`s (to the same
  or different addresses) at once; each gets its own response channel.
- **The `Config.amixenable` getter bug is fixed**: it reads
  `/config/amixenable` (what its own setter writes), not `/config/mute`
  (see `WIRE_PROTOCOL.md` §9 and the doc comment on `Config.AmixEnable`
  in `config.go`).

Run `go build ./...`, `go vet ./...`, and `go test ./...` (`-race` is
clean) from this directory. `examples/basic/main.go` mirrors the
`xair-api-python` README's usage example. CI (`.github/workflows/build.yaml`)
builds, checks that `go.mod`/`go.sum` are tidy, verifies module checksums,
and tests on every push and pull request, and additionally asks
pkg.go.dev to index the module on every published GitHub release.

## xair-api-python's OSC approach vs. Go OSC libraries

`xair-api-python` doesn't implement OSC itself — it builds on
[`python-osc`](https://pypi.org/project/python-osc/), using only a thin
slice of what that library offers: `OscMessageBuilder` to encode outgoing
messages, and a `Dispatcher` with a single **default handler** (not
per-address handlers) feeding a `BlockingOSCUDPServer` on a background
thread. Its request/response pattern (`send()` fire-and-forget, `query()`
= send + `time.sleep(delay)` + read a single shared "last packet
received" field) is entirely application code layered on top, not
something `python-osc` provides. Full details are in
[`WIRE_PROTOCOL.md`](WIRE_PROTOCOL.md) §§2–4.

The Go OSC ecosystem offers a spread of designs to build a client on top
of instead:

| Library | Language | Transport model | OSC feature coverage | Status (per GitHub, 2026) |
|---|---|---|---|---|
| [`hypebeast/go-osc`](https://github.com/hypebeast/go-osc) | Pure Go | `Client`/`Server` + `Dispatcher` with **per-address** handler registration (`AddMsgHandler`), address-pattern wildcards (`*`, `?`, `{,}`, `[]`) | Messages, bundles + timetags, types `i f s b h t d T F N` | Actively maintained, 232★ |
| [`scgolang/osc`](https://github.com/scgolang/osc) | Pure Go | `Conn` over UDP or Unix sockets, a `Worker`/dispatcher abstraction, and an explicit synchronous request/response ("ping-pong") pattern in its own docs | Aims for full OSC 1.0 spec compliance (messages, bundles, typed arguments) | Small, slow-moving, 36★ |
| [`mattkane218/go-osc`](https://github.com/mattkane218/go-osc) | **cgo** binding to the C `liblo` library | Wraps `liblo`'s client/server; build uses a pre-Go-modules `Makefile` (`$(GOROOT)/src/Make.inc`) | Whatever `liblo` supports | Unmaintained / build tooling predates modern Go, 14★ |
| [`goaudiovideo/osc`](https://github.com/goaudiovideo/osc) | Pure Go | No server/dispatcher at all — just a `Message()` function that encodes address+args to bytes | Message encoding only (no parser, no bundles) | Minimal, 3★, but already has a `behringer/x32` package with address constants |

### What xair-api-python's approach gets right

- **Minimal surface for a minimal need.** The mixer protocol itself never
  uses bundles, timetags, wildcard address matching, or any OSC type
  beyond `i`/`f`/`s` (`WIRE_PROTOCOL.md` §2, §10). Most of what
  `hypebeast/go-osc` and `scgolang/osc` implement — bundle/timetag
  support, the `T`/`F`/`N`/`b`/`h`/`d` argument types, wildcard address
  matching — is simply dead weight for this specific device protocol.
  A thin client is a legitimate choice here, not an oversight.
- **No native/cgo dependency.** `mattkane218/go-osc` requires linking
  against `liblo` via cgo and ships build tooling that predates Go
  modules — a real liability for cross-compilation and static binaries.
  xair-api-python's pure-library approach (and `hypebeast/go-osc`'s and
  `goaudiovideo/osc`'s pure-Go designs) avoid that class of problem
  entirely; a Go port should too.
- **Fire-and-forget sends need nothing fancy.** For the `SET` half of
  the protocol (§3), there's genuinely no reply to correlate, so a plain
  "encode and send a UDP datagram" is all any of these libraries'
  `Client.Send`/`Message()` equivalents need to provide.

### Where it falls short compared to what these libraries model

- **No address-based reply routing.** `python-osc`'s own `Dispatcher`
  supports registering a handler per address pattern — exactly what
  `hypebeast/go-osc`'s `AddMsgHandler` and `scgolang/osc`'s dispatcher
  do — but xair-api-python opts out of it via `set_default_handler` and
  instead overwrites one shared field with whatever datagram arrived
  most recently. That's the root cause of the race documented in
  `WIRE_PROTOCOL.md` §4: a `query()` can silently return the wrong
  value if any other datagram (an echo, or a reply to an overlapping
  request) lands during the sleep window. Building the Go client on a
  library that already does per-address dispatch removes this bug class
  almost for free.
- **`time.sleep(delay)` instead of waiting for the actual reply.**
  This wastes time when the mixer answers quickly and is simply wrong
  when it answers slowly (e.g. over a congested Wi-Fi link — the exact
  scenario the Python README warns about when recommending a larger
  `delay`). `scgolang/osc`'s ping-pong pattern and the general Go idiom
  of a per-request response channel with `select`/`context.Context`
  timeout give a "return as soon as the reply arrives, otherwise time
  out" client with lower *and* more predictable latency, and it
  composes with cancellation — something xair-api-python has no
  equivalent of.
- **A single shared response value is not concurrency-safe.** Because
  `_info_response` is one field written by the receiver thread and read
  by whichever call happens to be sleeping, xair-api-python's model
  implicitly assumes one in-flight query at a time. A Go port that
  copied this literally would need a mutex and would still serialize
  all gets. Per-address (or per-request-id) response channels — the
  pattern these dispatcher-based libraries are built for — allow
  multiple concurrent `Get`s to be issued from separate goroutines
  safely and answered independently.
- **No bundle/atomic-set support** — fine for parity with what the
  mixer itself sends, but if the Go client ever wants to fire several
  parameter changes atomically (the way some touch-OSC control surfaces
  batch fader moves in a bundle), `hypebeast/go-osc` and `scgolang/osc`
  already have the primitives; xair-api-python and `goaudiovideo/osc`
  do not.

### Takeaway for this port

Prefer a pure-Go, address-dispatching library (`hypebeast/go-osc` is the
most complete and actively maintained option) or a small hand-rolled
codec scoped like `goaudiovideo/osc`'s, and build the `Get`/`Set` client
API on top of **per-request, address-matched replies with a timeout**
rather than reproducing xair-api-python's shared-field-plus-sleep
pattern verbatim. Avoid `mattkane218/go-osc` (cgo/`liblo`, unmaintained
build system) — it works against the goal of a portable, easily
cross-compiled Go binary. `goaudiovideo/osc`'s `behringer/x32` package is
worth a direct look for cross-checking the X32 address constants against
`WIRE_PROTOCOL.md` §8, even though its lack of a server/dispatcher and
minimal maintenance make it unsuitable as the transport layer itself.

