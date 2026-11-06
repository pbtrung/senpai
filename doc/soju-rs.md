# soju-rs compatibility

Connect to soju-rs using your bouncer username and password. Leave the network
suffix out of the username to discover and connect to all your networks.
Manage networks with `/msg BouncerServ help`.

senpai negotiates these extensions automatically when available:

- `soju.im/bouncer-networks` and `soju.im/bouncer-networks-notify` discover
  networks and their changes.
- `soju.im/multiplex` attaches networks to one physical connection. Each network
  retains its own nickname, capabilities, channel state, batches, and history.
  Servers without multiplex support use separate connections per network.
- `soju.im/unread` reads unread and mention flags from `CHATHISTORY TARGETS`.
  History is fetched when a buffer is opened instead of for every conversation
  during startup. `draft/read-marker` synchronizes read progress across clients.
  A server snapshot indicates that a conversation has mentions, not their count.
- `draft/multiline` sends pasted chat text and long messages in batches, within
  the server's advertised byte and line limits. Incoming messages retain their
  line breaks, including in history and search results. Text exceeding a batch's
  limits is sent as multiple messages. Without this capability, outgoing text
  is split into ordinary IRC messages.
- `draft/chathistory`, `draft/event-playback`, `soju.im/search`, and
  `draft/metadata-2` provide history, server-side search, and pinned/muted buffers.
  File uploads use the server's `soju.im/FILEHOST` ISUPPORT token.

Capabilities depend on the upstream network as well as the bouncer. In
particular, soju-rs may add or remove multiline support when an upstream
connects or disconnects. No special senpai configuration is required.

## Testing with a prebuilt release

Download and extract a Linux release archive from
[soju-rs releases](https://github.com/pbtrung/soju-rs/releases), and verify its
checksum against the release's `SHA256SUMS`. Keep `soju` and `sojuctl` together
in the extracted directory, then run from the senpai repository:

```sh
SOJU_RS_BINARY=/absolute/path/to/soju go test ./... -count=1
```

The integration test runs that binary with temporary SQLite storage, a local
admin socket, two simulated upstream IRC networks, and a local client connection.
It does not use an existing bouncer account or contact public IRC networks.
It covers authentication, capability negotiation, multiplex attachment,
multiline traffic, history, unread/mention snapshots, read markers, metadata,
and detaching one network while keeping the connection alive.

Without `SOJU_RS_BINARY`, the integration test is skipped; protocol and UI unit
tests still run with `go test ./...`.
