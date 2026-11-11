package senpai

import (
	"fmt"
)

var (
	errOffline      = fmt.Errorf("you are disconnected from the server, retry later")
	errNotSupported = fmt.Errorf("command not supported by the server; try using latest soju")
)

const maxArgsInfinite = -1

type command struct {
	AllowHome bool
	MinArgs   int
	MaxArgs   int
	Usage     string
	Desc      string
	Handle    func(app *App, args []string) error // nil = passthrough
}

type commandSet map[string]*command

var commands commandSet

func init() {
	commands = commandSet{
		"HELP": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[command]",
			Desc:      "show the list of commands, or how to use the given one",
			Handle:    commandDoHelp,
		},
		"BOUNCER": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<bouncer message>",
			Desc:      "send command to the bouncer service (only works with soju); e.g. /bouncer help",
			Handle:    commandDoBouncer,
		},
		"JOIN": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<channels> [keys]",
			Desc:      "join a channel",
			Handle:    commandDoJoin,
		},
		"ME": {
			MinArgs: 1,
			MaxArgs: 1,
			Usage:   "<message>",
			Desc:    "send an action (reply to last query if sent from home)",
			Handle:  commandDoMe,
		},
		"NP": {
			Desc:   "send the current song that is being played on the system",
			Handle: commandDoNP,
		},
		"UPLOAD": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[file path]",
			Desc:      "upload a local file to the bouncer, or the current clipboard content if no path is given",
			Handle:    commandDoUpload,
		},
		"SCREENSHOT": {
			AllowHome: true,
			Desc:      "take and upload a screenshot to the bouncer",
			Handle:    commandDoScreenshot,
		},
		"MSG": {
			AllowHome: true,
			MinArgs:   2,
			MaxArgs:   2,
			Usage:     "<target> <message>",
			Desc:      "send a message to the given target",
			Handle:    commandDoMsg,
		},
		"MOTD": {
			AllowHome: true,
			Desc:      "show the message of the day (MOTD)",
		},
		"NAMES": {
			Desc:   "show the member list of the current channel",
			Handle: commandDoNames,
		},
		"NICK": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<nickname>",
			Desc:      "change your nickname",
			Handle:    commandDoNick,
		},
		"OPER": {
			AllowHome: true,
			MinArgs:   2,
			MaxArgs:   2,
			Usage:     "<username> <password>",
			Desc:      "log in to an operator account",
		},
		"MODE": {
			AllowHome: true,
			MaxArgs:   maxArgsInfinite,
			Usage:     "[<nick/channel>] [<flags>] [args]",
			Desc:      "change channel or user modes",
			Handle:    commandDoMode,
		},
		"PART": {
			AllowHome: true,
			MaxArgs:   2,
			Usage:     "[channel] [reason]",
			Desc:      "part a channel",
			Handle:    commandDoPart,
		},
		"QUERY": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "[nick] [message]",
			Desc:      "opens a buffer to a user",
			Handle:    commandDoQuery,
		},
		"QUIT": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[reason]",
			Desc:      "quit senpai",
			Handle:    commandDoQuit,
		},
		"QUOTE": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<raw message>",
			Desc:      "send raw protocol data",
			Handle:    commandDoQuote,
		},
		"LIST": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[pattern]",
			Desc:      "list public channels",
			Handle:    commandDoList,
		},
		"REPLY": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<message>",
			Desc:      "reply to the last query",
			Handle:    commandDoR,
		},
		"TOPIC": {
			MaxArgs: 1,
			Usage:   "[topic]",
			Desc:    "show or set the topic of the current channel",
			Handle:  commandDoTopic,
		},
		"MUTE": {
			Desc:   "mute the current channel (preventing color on new non-highlight messages)",
			Handle: commandDoMute,
		},
		"UNMUTE": {
			Desc:   "unmute the current channel",
			Handle: commandDoUnmute,
		},
		"PIN": {
			Desc:   "pin the current channel (moving it to the top of the channel list)",
			Handle: commandDoPin,
		},
		"UNPIN": {
			Desc:   "unpin the current channel",
			Handle: commandDoUnpin,
		},
		"BUFFER": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<index|name>",
			Desc:      "switch to the buffer at the position or containing a substring",
			Handle:    commandDoBuffer,
		},
		"WHOIS": {
			AllowHome: true,
			MinArgs:   0,
			MaxArgs:   1,
			Usage:     "<nick>",
			Desc:      "get information about someone who is connected",
			Handle:    commandDoWhois,
		},
		"WHOWAS": {
			AllowHome: true,
			MinArgs:   0,
			MaxArgs:   1,
			Usage:     "<nick>",
			Desc:      "get information about someone who is disconnected",
			Handle:    commandDoWhowas,
		},
		"INVITE": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<name> [channel]",
			Desc:      "invite someone to a channel",
			Handle:    commandDoInvite,
		},
		"KICK": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   3,
			Usage:     "<nick> [channel] [message]",
			Desc:      "eject someone from the channel",
			Handle:    commandDoKick,
		},
		"BAN": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<nick> [channel]",
			Desc:      "ban someone from entering the channel",
			Handle:    commandDoBan,
		},
		"UNBAN": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<nick> [channel]",
			Desc:      "remove effect of a ban from the user",
			Handle:    commandDoUnban,
		},
		"CONNECT": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   3,
			Usage:     "<target server> [<port> [remote server]]",
			Desc:      "connect a server to the network",
		},
		"SQUIT": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<server> [comment]",
			Desc:      "disconnect a server from the network",
		},
		"KILL": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<nick> [message]",
			Desc:      "eject someone from the server",
		},
		"SEARCH": {
			MaxArgs: 1,
			Usage:   "<text>",
			Desc:    "search messages in a target",
			Handle:  commandDoSearch,
		},
		"AWAY": {
			AllowHome: true,
			MinArgs:   0,
			MaxArgs:   1,
			Usage:     "[message]",
			Desc:      "mark yourself as away (use /BACK to unset away)",
			Handle:    commandDoAway,
		},
		"BACK": {
			AllowHome: true,
			Desc:      "mark yourself as back from being away",
			Handle:    commandDoBack,
		},
		"SHRUG": {
			Desc:    "send a shrug to the current channel ¯\\_(ツ)_/¯",
			MaxArgs: maxArgsInfinite,
			Handle:  commandDoShrug,
		},
		"TABLEFLIP": {
			Desc:   "send a tableflip to the current channel (╯°□°)╯︵ ┻━┻",
			Handle: commandDoTableFlip,
		},
		"VERSION": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[target]",
			Desc:      "query the server software version",
		},
		"ADMIN": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[target]",
			Desc:      "query the server administrative information",
		},
		"LUSERS": {
			AllowHome: true,
			Desc:      "query the server user information",
		},
		"TIME": {
			AllowHome: true,
			MaxArgs:   1,
			Usage:     "[target]",
			Desc:      "query the server local time",
		},
		"STATS": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   2,
			Usage:     "<query> [target]",
			Desc:      "query server statistics",
		},
		"INFO": {
			AllowHome: true,
			Desc:      "query server information",
		},
		"REHASH": {
			AllowHome: true,
			Desc:      "make the server reload its configuration",
		},
		"RESTART": {
			AllowHome: true,
			Desc:      "make the server restart",
		},
		"LINKS": {
			AllowHome: true,
			Desc:      "query the servers of the network",
		},
		"WALLOPS": {
			AllowHome: true,
			MinArgs:   1,
			MaxArgs:   1,
			Usage:     "<text>",
			Desc:      "broadcast a message to all users",
		},
	}
}
