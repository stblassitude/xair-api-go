# XAir / X32 Wire Protocol Specification

This document specifies the wire protocol spoken between `xair-api-python`
(the reference implementation) and Behringer X-Air / Midas MR / Behringer X32
digital mixers, as a basis for a from-scratch Go implementation. It describes
what actually goes on the network, not the Python object model — but where
the Python client's behavior *is* the de-facto protocol (e.g. its lack of
reply correlation), that is called out explicitly since a Go port must
either replicate it or deliberately improve on it.

Everything here was derived by reading `xair-api-python/xair_api/*.py`. Two
public references cover the mixer side of the protocol in more depth:

- XAir OSC commands: https://behringer.world/wiki/doku.php?id=x-air_osc
- X32 OSC commands: https://wiki.munichmakerlab.de/images/1/17/UNOFFICIAL_X32_OSC_REMOTE_PROTOCOL_%281%29.pdf

## 1. Transport

- **Protocol:** UDP, unicast, no TLS/encryption, no authentication.
- **Ports:** mixer listens on `10024` for X-Air/MR-series, `10023` for X32.
  (`xair.py`, `init_xair`/`init_x32` default kwargs.)
- **Socket model:** the client creates a single UDP socket bound to an
  ephemeral local port (`('', 0)`) and treats the mixer's `(ip, port)` as
  the implicit peer for every send. There is no connect()/handshake at the
  socket level — any UDP datagram received on that socket, from anyone, is
  processed identically.
- **Session/handshake:** none. OSC has no concept of a session. The mixer
  does not need to be told "hello" before accepting commands; the client
  only sends `/xinfo` first as an application-level sanity check (§5).
- **Keep-alive:** none used by this client. (Real X-Air/X32 firmware also
  supports an `/xremote` subscription that must be renewed every <10s to
  keep receiving unsolicited meter/parameter pushes; this client never
  sends `/xremote` and relies purely on synchronous request/reply. A Go
  port that wants push updates would need to add this independently — it
  is not part of the protocol this client speaks today.)
- **Teardown:** client simply stops its receive loop and closes the socket.
  No disconnect message exists.

## 2. OSC packet format

Every datagram is a single, non-bundled OSC 1.0 message (built/parsed via
`python-osc`, i.e. `OscMessageBuilder` on send and a `Dispatcher` on
receive). No OSC bundles, no timetags, no blobs are ever used by this
client.

A message on the wire is:

```
<address-pattern> <type-tag-string> <argument>*
```

Encoding rules (standard OSC 1.0):

- **Address pattern**: ASCII string starting with `/`, NUL-terminated,
  then padded with additional NUL bytes so the total length (including
  terminator) is a multiple of 4.
- **Type tag string**: starts with `,`, followed by one tag character per
  argument, NUL-terminated and NUL-padded to a multiple of 4 the same way.
  This client only ever emits/consumes:
  - `i` — int32, big-endian two's complement
  - `f` — float32, big-endian IEEE-754
  - `s` — OSC string (same NUL-terminated/padded encoding as the address)
  A message with **zero arguments** has type tag string `","` and no
  argument bytes at all (this is how a GET/query is expressed — see §3).
- **Arguments**: concatenated in order, each encoded per its type tag as
  above, no padding between them beyond what string encoding already
  requires.

There is no length prefix on UDP (the datagram boundary is the message
boundary) and no checksum beyond what UDP/IP already provide.

## 3. Message semantics: GET vs SET

The protocol has exactly one message shape; the mixer infers *GET* vs
*SET* purely from whether arguments are present:

- **GET** (query current value): send an OSC message to a parameter's
  address with **no arguments** (`send.address, args=[]`). The mixer
  replies with an OSC message to the *same address*, carrying the
  current value(s) as argument(s).
- **SET** (change value): send an OSC message to the address with
  **one argument** (the new value, typed `i`/`f`/`s` as appropriate for
  that parameter). The mixer applies it. This client does not wait for
  or otherwise process an acknowledgement.

`adapter`/`strip`/`bus`/etc. getter/setter helpers in the Python code
(`self._remote.query(...)` / `self._remote.send(...)`) are exactly these
two shapes — see `IStrip.getter`/`.setter` in `strip.py` as a representative
example, repeated near-verbatim in `bus.py`, `fx.py`, `rtn.py`, `lr.py`,
`dca.py`, `headamp.py`, `config.py`.

## 4. Client request/response algorithm (application-level, not OSC-mandated)

This is *not* part of OSC itself, but it is exactly what this client does
and a Go port needs to decide whether to copy or improve it:

1. A background thread runs `BlockingOSCUDPServer.serve_forever()` on the
   client's socket for the lifetime of the connection. Its dispatcher has
   a single **default handler** matching every possible address — i.e.
   the client does *not* route replies by address. Whatever the most
   recently received datagram was gets stored, unconditionally, in a
   single shared field (`self._info_response`), overwriting the previous
   value (`xair.py: msg_handler`).
2. `send(addr, param=None)` — fire-and-forget: build and send one OSC
   message, no waiting.
3. `query(addr)` — send a GET (i.e. `send(addr)` with no param) then
   `time.sleep(self._delay)` (default `0.02s`) and return whatever is
   currently sitting in the shared `_info_response` field.
4. **Consequence / known race**: because there is no address matching and
   no per-request correlation ID (OSC has neither concept built in), if
   *any* other datagram arrives from the mixer during that 20ms window —
   including an unrelated echo, or a reply to a previous overlapping
   request — `query()` will happily return the wrong value. This client
   avoids the problem in practice only by being effectively
   single-threaded/serialized (callers issue one query at a time and the
   default delay is tuned to normally be long enough) and by not enabling
   `/xremote` push updates. A Go implementation may reasonably choose to
   fix this (e.g. match on address, use a real request/response
   correlation with channels/futures) rather than reproduce the race,
   but should preserve the observable timing behavior (a configurable
   settle delay) for compatibility with real hardware over Wi-Fi, per the
   README's note that wired connections can safely use a shorter delay.

## 5. Connection establishment / validation

There is no protocol-level connect. The client validates reachability by:

1. Sending a GET to `/xinfo` (no arguments) and calling `query`.
2. If the response tuple is falsy (empty — i.e. nothing arrived within the
   delay), it's treated as a failed attempt.
3. This is retried in a loop for up to `connect_timeout` seconds (default
   `2`), via the `util.timeout` decorator, raising
   `XAirRemoteConnectionTimeoutError(ip, port)` if it never succeeds.
4. On success, `info_response[0]` and `info_response[2]` are logged as
   the mixer's address and name respectively — consistent with the known
   `/xinfo` reply shape `(ip: string, name: string, model: string,
   fw-version: string)` documented for X-Air/X32 hardware.

## 6. Value encoding conventions (application layer over OSC types)

The wire only ever carries `i` (int32), `f` (float32 normalized), or `s`
(string). Higher-level meaning is entirely client-side convention:

- **Boolean**: `i`, `0` or `1`.
- **Plain integer**: `i`, raw value (e.g. `Config.color`, `Group.dca`,
  `EQBand.type`).
- **Enumerated choice**: `i`, an index into a fixed, ordered tuple of
  option names defined only in the client (never sent as a string). E.g.
  `Gate.mode` opts = `('exp2','exp3','exp4','gate','duck')`,
  `Dyn.mode` = `('comp','exp')`, `Dyn.det` = `('peak','rms')`,
  `Dyn.env` = `('lin','log')`, `EQ.mode` = `('peq','geq','teq')`,
  `Dyn.ratio` = `(1.1,1.3,1.5,2.0,2.5,3.0,4.0,5.0,7.0,10,20,100)` (index
  sent as `i`, but the *decoded* value is itself one of these numbers,
  not the index).
- **Name/string fields**: `s` verbatim (e.g. `Config.name`).
- **Normalized float parameters**: `f` in **`0.0`–`1.0`** on the wire,
  mapped to/from a real-world range client-side via one of two curves
  (`util.py`):
  - **Linear**: `real = min + (max-min) * wire`;
    `wire = (real-min) / (max-min)`
  - **Logarithmic**: `real = min * exp(ln(max/min) * wire)`;
    `wire = ln(real/min) / ln(max/min)`

  Applied per-parameter with these bounds (min, max, curve):

  | Parameter | Range | Curve |
  |---|---|---|
  | Preamp `usbtrim` | -18..18 dB | linear |
  | Preamp `highpassfilter` | 20..400 Hz | log |
  | Gate/Dyn `threshold` | -80..0 / -60..0 dB | linear |
  | Gate `range` | 3..60 | linear |
  | Gate/Dyn `attack` | 0..120 ms | linear |
  | Gate/Dyn `hold` | 0.02..2000 ms | log |
  | Gate/Dyn `release` | 5..4000 ms | log |
  | Gate/Dyn `filterfreq` | 20..20000 Hz | log |
  | Dyn `knee` | 0..5 | linear |
  | Dyn `mgain` | 0..24 dB | linear |
  | Dyn `mix` | 0..100 % | linear |
  | EQBand `frequency` | 20..20000 Hz | log |
  | EQBand `gain` / GEQ `slider_*` | -15..15 dB | linear |
  | EQBand `quality` | 0.3..10, wire = `1 - log_set(0.3,10,val)` | log (inverted) |
  | Automix `weight` | -12..12 dB | linear |
  | Config.Monitor `sourcetrim` | -18..18 dB | linear |
  | Config.Monitor `dimgain` | -40..0 dB | linear |

- **Fader/level (`Mix.fader`, `Send.level`, `Config.Monitor.level`)**:
  also `f` in `0.0`–`1.0` on the wire, but via a distinct **piecewise dB
  taper** (`util.db_from`/`util.db_to`), not the generic lin/log curves:

  Decode (wire → dB):
  ```
  wire >= 1.0    -> 10.0
  wire >= 0.5    -> 40*wire - 30      (rounded to 0.1)
  wire >= 0.25   -> 80*wire - 50      (rounded to 0.1)
  wire >= 0.0625 -> 160*wire - 70     (rounded to 0.1)
  wire >= 0.0    -> 480*wire - 90     (rounded to 0.1)
  wire < 0.0     -> -90.0             (i.e. -inf, floor)
  ```
  Encode (dB → wire), exact inverse breakpoints:
  ```
  dB >= 10   -> 1.0
  dB >= -10  -> (dB+30)/40
  dB >= -30  -> (dB+50)/80
  dB >= -60  -> (dB+70)/160
  dB >= -90  -> (dB+90)/480
  dB < -90   -> 0.0
  ```
  This must be reproduced bit-for-bit (same breakpoints/formulas) for a
  compatible Go client, since it's specific to how these mixers represent
  fader position internally.

## 7. Device parameter tables (`kinds.py`)

Iteration bounds for building each address space (1-based indices on the
wire in all cases):

| Kind | strips (`/ch`) | buses | DCA groups | FX slots | aux returns | matrix | headamps |
|---|---|---|---|---|---|---|---|
| X32 | 32 | 16 | 8 | 8 | 8 | 6 | 127 |
| MR18 / XR18 | 16 | 6 | 4 | 4 | 1 (fixed) | — | 16 (= num strips) |
| XR16 | 16 | 4 | 4 | 4 | 1 (fixed) | — | 16 |
| XR12 | 12 | 2 | 4 | 4 | 1 (fixed) | — | 12 |

Note X32 vs X-Air/MR are not just parameter-count variants — several
address roots and zero-padding widths differ between the two families
(§8). The Python client models this as two different code paths
(`init_x32` vs `init_xair` in `xair.py`), not a single generic template.

## 8. Address space

All addresses are rooted at `/`. Segments below are appended with a
literal `/`. Indices are 1-based unless noted. **Padding width differs by
mixer family for the same logical entity** — this is the single most
important gotcha to replicate faithfully:

| Entity | X-Air / MR address | X32 address | Count source |
|---|---|---|---|
| Input strip | `/ch/NN` (2-digit) | `/ch/NN` (2-digit) | `num_strip` |
| Bus | `/bus/N` (no padding) | `/bus/NN` (2-digit) | `num_bus` |
| Main stereo | `/lr` | `/main/st` | fixed, 1 |
| Main mono | *(none)* | `/main/m` | fixed, 1 (X32 only) |
| Matrix | *(none)* | `/mtx/NN` (2-digit) | `num_matrix` (X32 only) |
| DCA group | `/dca/N` (no padding) | `/dca/N` (no padding) | `num_dca` |
| FX (effect engine) | `/fx/N` (no padding) | `/fx/N` (no padding) | `num_fx` |
| FX send (pre-FX bus) | `/fxsend/N` (no padding) | *(none — not modeled)* | `num_fx` |
| FX return | `/rtn/N` (no padding) | `/fxrtn/NN` (2-digit) | `num_fx` |
| Aux return | `/rtn/aux` (single, fixed) | `/auxin/NN` (2-digit, array) | 1 vs `num_auxrtn` |
| Head amp / preamp | `/headamp/NN` (2-digit; count = `num_strip`) | `/headamp/NNN` (3-digit; count = `num_headamp`) | see above |
| Global config | `/config` | `/config` | fixed, 1 |
| Device info (GET only) | `/xinfo` | `/xinfo` | fixed |

Sub-trees appended under each channel-like root above (identical suffixes
on both families unless noted):

- `/config` — `name` (s), `color` (i), `insrc` (i), `rtnsrc` (i)
- `/preamp` — `rtntrim` (f, lin -18..18), `rtnsw` (i bool), `invert`
  (i bool), `hpon` (i bool), `hpf` (f, log 20..400)
- `/gate` — `on` (i bool), `mode` (i enum, 5 opts), `thr` (f, lin -80..0),
  `range` (f, lin 3..60), `attack` (f, lin 0..120), `hold` (f, log
  0.02..2000), `release` (f, log 5..4000), `keysrc` (i), `filter/on`
  (i bool), `filter/type` (i), `filter/f` (f, log 20..20000)
- `/dyn` — `on`, `mode` (2 opts), `det` (2 opts), `env` (2 opts), `thr`
  (lin -60..0), `ratio` (i, 12-value lookup table, not linear), `knee`
  (lin 0..5), `mgain` (lin 0..24), `attack` (lin 0..120), `hold` (log
  0.02..2000), `release` (log 5..4000), `mix` (lin 0..100), `keysrc` (i),
  `auto` (i bool), `filter/on`, `filter/type`, `filter/f` (log 20..20000)
- `/insert` — `on` (i bool), `sel` (i)
- `/eq` — `on` (i bool), `mode` (i enum, 3 opts: peq/geq/teq); plus
  `/eq/1`..`/eq/4` (4-band: strip, aux, fx-return) or `/eq/1`..`/eq/6`
  (6-band: bus, main/lr) each with `type` (i), `f` (f, log 20..20000),
  `g` (f, lin -15..15), `q` (f, log 0.3..10, *inverted*: wire = `1 -
  log_set(0.3,10,q)`)
- `/geq/<band>` — 31 fixed graphic-EQ bands, `f` (lin -15..15). Band
  address tokens (note `_`→`.` mapping, e.g. Python attribute
  `slider_31_5` addresses `/geq/31.5`): `20 25 31.5 40 50 63 80 100 125
  160 200 250 315 400 500 630 800 1k 1k25 1k6 2k 2k5 3k15 4k 5k 6k3 8k
  10k 12k5 16k 20k` (bus and main/lr only)
- `/mix` — `on` (i bool), `fader` (f, dB taper §6), `lr` (i bool); plus
  `/mix/NN` (2-digit, one per bus+fx-return, i.e. send) — `level` (f, dB
  taper)
- `/grp` — `dca` (i, bitmask/index 0..15), `mute` (i, 0..15)
- `/automix` — `group` (i, 0..2), `weight` (f, lin -12..12)

DCA-specific (no `/mix` subtree): `/dca/N/on` (i bool), `/dca/N/config/name`
(s), `/dca/N/config/color` (i).

Head-amp-specific: `/headamp/../gain` (f, lin -12..60), `/headamp/../phantom`
(i bool).

Global `/config` subtree:

- `/config/mute/1`..`/config/mute/4` — `on` (i bool), one address segment
  per mute group, always exactly 4 regardless of mixer kind
- `/config/solo` — `level` (f, dB taper), `source` (i), `sourcetrim` (f,
  lin -18..18), `chmode` (i bool), `busmode` (i bool), `dimatt` (f, lin
  -40..0, exposed as `dimgain`), `dim` (i bool), `mono` (i bool), `mute`
  (i bool), `dimfpl` (i bool)
- `/config/linkcfg/eq`, `/config/linkcfg/dyn`, `/config/linkcfg/fdrmute`
  — `i` bool
- `/config/chlink/<i>-<i+1>` for `i` = 1,3,5,... up to `num_strip-1` — `i`
  bool (stereo-link pairs of input strips)
- `/config/buslink/<i>-<i+1>` for `i` = 1,3,5,... up to `num_bus-1` — `i`
  bool (stereo-link pairs of buses)
- `/config/amixenable`, `/config/amixlock` — `i` bool

## 9. Known quirk in the reference implementation

`Config.amixenable` (`config.py`) **sets** `/config/amixenable` but
**gets** `/config/mute` — the getter reads the wrong address (looks like a
copy-paste bug from the neighboring `Monitor.mute` property). A faithful
byte-for-byte port would reproduce this; a corrected port should get from
`/config/amixenable` instead. Flagging it here so the discrepancy is a
deliberate choice in the Go implementation rather than an accidental
divergence.

## 10. Out of scope / not implemented by this client

These are real capabilities of the mixer's OSC surface (per the public
docs linked above) that `xair-api-python` never uses, so they are not
covered by this spec and would need separate protocol research if a Go
port wants them:

- `/xremote` subscription (and its ~10s renewal requirement) for
  unsolicited metering/parameter-change push
- Meter block addresses (`/meters/...`)
- Snapshot/scene/cue OSC commands
- OSC bundles / timetags
