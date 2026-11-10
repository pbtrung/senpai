# senpai

A modern terminal IRC client designed for bouncers. Features include server-side
chat history and message search, automatic connections to multiple networks,
typing indicators, and file uploads with drag and drop. These features require
support from your server or bouncer; senpai does not keep local chat logs.

With [soju-rs](https://github.com/pbtrung/soju-rs), senpai supports unread and
mention indicators, multiple networks over one connection, and multiline
messages. See the [compatibility notes](doc/soju-rs.md) for details.

## Installation

To build from source, install Go 1.26 or later, Make, and
[scdoc](https://git.sr.ht/~sircmpwn/scdoc) for the manual pages, then run:

```sh
git clone https://github.com/pbtrung/senpai.git
cd senpai
make
sudo make install
```

## Usage

Start senpai from your terminal:

```sh
senpai
```

The setup assistant will guide you through configuration on your first run.
Once connected, join a channel with `/join #channel`, type a message to chat,
and use `/help` to see available commands. Exit with `/quit`.

Use `Ctrl+N` and `Ctrl+P` to switch between buffers, and `Tab` for completion.
The conversation list and read markers are restored between runs. Each buffer
keeps up to 2,000 messages in memory; message contents are fetched from the
server and are not saved locally. Set `transient true` to disable saved state.

To use a different configuration file:

```sh
senpai -config /path/to/senpai.scfg
```

See the [user manual](doc/senpai.1.scd) for commands and keyboard shortcuts, and
the [configuration reference](doc/senpai.5.scd) for configuration options.
