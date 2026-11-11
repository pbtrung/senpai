# Code audit and refactor — 2026-11-11

Scope: application lifecycle, networking and IRC state, history and unread
tracking, command handling, terminal rendering and editor behavior, image and
file transfers, configuration persistence, spell checking, and local desktop
integrations. This is a source review with targeted regressions and a short fuzz
run, not a proof that every behavior is correct. Remaining findings are listed
explicitly below.

## Fixed findings

Severity describes the consequence of the original behavior. Several protocol
failures require malformed input from the connected IRC server.

| Area | Finding and consequence | Change and regression coverage |
| --- | --- | --- |
| Shutdown — high | Signal-triggered `Close` could mutate/close sessions while the event loop used them; producers could remain blocked after shutdown. | Cancel producers through a stop channel, keep cleanup with the running event loop, close queued session arrivals, and clean up once. Tests cover concurrent producers and active session sends. |
| Completion — high | Expired entries shortened the stored slice while iteration continued over the original local length, revisiting stale entries and eventually corrupting queue handling. | Shorten both views together; test consecutive expired entries followed by a live completion. |
| Protocol — high | Short numeric replies indexed absent parameters and could panic. | Validate numeric parameters before dispatch; exercise all 1,000 numeric commands with zero and one parameter. |
| Protocol — high | An empty `batch` tag could match an unopened target snapshot and write to nil maps. Invalid BATCH identifiers were accepted. | Reject missing/signless IDs and exclude empty batch tags from routing. |
| Protocol — high | NAMES entries containing only membership prefixes produced nil user prefixes. | Ignore prefix-only entries; exercise channel membership parsing. |
| Protocol — high | Negative or huge LINKS hop counts reached `strings.Repeat`. | Validate the range before rendering; test negative and excessive values. |
| Transport — high | After a write failure, output producers could fill the queue and prevent the event loop from receiving the disconnect. | Close the failed connection and drain output until the owner closes it; test more than the queue capacity after disconnect. |
| Media — high | Image decoding could allocate from unbounded remote dimensions/body sizes. | Enforce 20 MiB encoded and 32-megapixel limits before decoding pixel storage; test both limits. |
| Configuration — high | Migration used ordinary file creation for password-bearing configuration, missed new destination directories, and ignored close errors. | Copy through a 0600 temporary file in a 0700-created directory; rename after successful copy/close. Test permissions and preservation of the destination on failure. |
| History — medium | The first message initialized only the lower bound; reconnect/history requests could lack a usable upper bound. | Initialize endpoints independently; test first and out-of-order messages. |
| Unread — medium | Selecting/focusing a buffer discarded its unloaded unread timestamp before history was read. | Preserve snapshot progress until covered by a marker; test partial reads and mark-all-read after selection. |
| Unread — medium | Local status-only lines could advance a read marker. | Select a readable line before updating progress. |
| Channel state — medium | Repeated privilege grants duplicated prefixes; removing an absent privilege erased other privileges. | Make mode updates idempotent and preserve unaffected membership. |
| Protocol — medium | Repeated spaces created empty parameters and shifted handlers' arguments. | Skip separator whitespace while preserving trailing text. |
| Spell checking — medium | The writer stalled waiting for a configuration response after a text change; closing its queue raced producers; invalid frame lengths could panic/allocate excessively. | Use cancellation, process changes independently, bound frame lengths, reap the subprocess, and remove its dictionary. Tests cover successive changes and shutdown/frame errors. |
| Media — medium | Background image decoding read terminal dimensions concurrently with resizing; zero cell sizes could divide by zero, and equal scaling ratios failed to resize. | Resize on the UI thread, clamp cell/output dimensions, and use the smaller scale. |
| HTTP — medium | Upload responses were never closed, and preview GET errors/relative embed URLs were mishandled. | Close all responses, check status codes, resolve references against the redirected page URL; use response-tracking and HTTP-server regressions. |
| Desktop — medium | Notification signals used an incorrect sender filter, portal responses used the parent path rather than request paths, and payload assertions could panic. | Correct match rules and validate payloads; test notification/screenshot events and malformed values. |
| Local integration — medium | A stale socket stopped discovery before a live instance; RPC waits were unbounded. | Continue to other sockets with deadlines and promptly close clients; test stale-before-live discovery. |

## Requested behavior

- **Alt+H** invokes the configurable `buffer-mark-all-read` action. It clears all
  real buffers across networks, including muted and unloaded conversations,
  closes notifications, persists read progress, and sends server markers where
  supported. It keeps search overlays and uses known message timestamps instead
  of the local clock. Tests cover offline persistence, connected synchronization,
  stale snapshots, repeated actions, and subsequent new messages.
- Truncated nicknames retain every grapheme that fits the nickname column.
  **`+` occupies the first separator cell**, leaving one space before the message.
  Full nickname click targets remain intact. Tests include wide characters,
  combining marks, exact fits, and one-cell columns.

## Refactor

The refactor is a separate commit from the behavior changes. Packages and public
APIs remain in place. A token comparison against the preceding commit verified
705 existing declarations unchanged; the two large dispatchers were split into
responsibility-specific handlers.

| Original file | Baseline lines | Refactored lines | Extracted responsibilities |
| --- | ---: | ---: | --- |
| `app.go` | 2,873 | 237 | Event loop, networking, input, keys, history, formatting, media, highlights, network/history event handling, version metadata |
| `irc/session.go` | 2,428 | 443 | Registration, channel state, batches, metadata, numeric replies, outgoing commands, history requests, features, SASL |
| `ui/buffers.go` | 1,456 | 472 | Line layout, history storage, unread/read progress, list rendering, timeline rendering |
| `ui/ui.go` | 1,130 | 304 | Buffer controls, editor forwarding, drawing, image handling |
| `commands.go` | 1,199 | 366 | Registry, parsing/dispatch, help, buffer commands, media commands |
| `cmd/senpai/main.go` | 478 | 236 | Configuration helpers, state persistence, local integrations |

Editor completion, search, and drawing also have dedicated files. New files stay
within their existing packages so the split does not introduce package cycles or
new exported abstractions solely for file organization.

## Remaining findings

These are source-review findings, not failures found by the fuzz run. They remain
open and should guide follow-up work.

1. **Medium: UI lookup does not use negotiated IRC case mapping.**
   `ui/buffers.go: BufferList.at` compares `strings.ToLower`, while the session and
   persisted buffer store use IRC mappings. On RFC1459 networks, aliases such as
   `nick[` and `nick{` can create distinct UI entries or miss updates. Correcting
   this requires per-network mapping and a policy for merging existing buffer
   history, selection, and markers when aliases collide.
2. **Medium: history deduplication lacks stable message identity.**
   `history.go: bound.Compare` truncates timestamps to seconds and compares body
   text at boundaries. Distinct messages with equal text/time can be treated as
   duplicates, while differing messages within a boundary second can be inserted
   on the wrong side. A robust solution should carry server message IDs through
   events and lines, with a defined fallback for servers without IDs.
3. **Medium, potentially high with a malicious server: ordinary batch storage is
   unbounded.** `irc/session_batches.go` accumulates history/search/network-list
   batches until their closing message. Multiline batches and final UI history
   have limits, but unfinished ordinary batches can still consume memory. Add
   count/byte budgets with request cancellation and retry semantics; silently
   truncating a batch could otherwise mark incomplete history as complete.
4. **Low: first-run configuration can loop on EOF.** Required hostname/nickname
   loops in `cmd/senpai/main.go` ignore input errors. Closed standard input with
   no default can cause a busy loop. Required input should return a clear error
   when EOF arrives.
5. **Low: spell-check diagnostics do not track document versions.**
   `harper.go` accepts diagnostic ranges without matching a document version to
   the current input. A delayed response can briefly underline the wrong text.
   Version-aware delivery and filtering should be tested with delayed responses.

## Validation and limits

Passed:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...` on Linux
- `GOOS=darwin GOARCH=arm64 go build ./...`
- `GOOS=windows GOARCH=amd64 go build ./...`
- `go test ./irc -run '^$' -fuzz '^FuzzSessionMessage$' -fuzztime=20s -parallel=2`
  — 651,569 executions without a crash; this target handles one parsed message
  per fresh session and does not explore arbitrary multi-message state sequences.
- `gofmt -l .` and `git diff --check` — clean.

Final statement coverage from `go test -cover ./...`: application **15.2%**,
CLI **23.2%**, IRC **42.9%**, UI **23.1%**, varlink service **81.2%**. The events
package contains data types and reports **0.0%**. Passing tests leave substantial
untested rendering and interactive behavior.

The optional soju-rs integration test was skipped because `SOJU_RS_BINARY` was
not supplied. Cross-platform builds verify compilation, not desktop runtime
behavior. D-Bus tests validate signal handling without a live notification/portal
service; no live terminal visual review or installed Harper process was used.
`scdoc` was unavailable, so manual sources were edited but generated manual pages
were not rebuilt.
