package irc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (s *Session) handleReplyMessage(msg Message) (Event, error) {
	switch msg.Command {
	case errMonlistisfull:
		// silence monlist full error, we don't care because we do it best-effort

	case rplAway:
		// we display user away status, we don't care about automatic AWAY replies

	case rplYourhost, rplCreated:
		// useless conection messages

	case rplAdminme:
		// useless admin info header

	case rplMotdstart, rplEndofmotd, errNomotd:
		// useless motd related messages

	case rplHostHidden:
		// useless host message

	case rplEndofstats:
		// useless stats delimiter

	case rplEndofwhois:
		// useless whois delimiter

	case rplListstart:
		// useless list delimiter

	case rplEndofinvitelist, rplEndofinvexlist:
		// useless invite list delimiter

	case rplEndofexceptlist:
		// useless exception list delimiter

	case rplEndoflinks:
		// useless links delimiter

	case rplEndofbanlist:
		// useless ban list delimiter

	case rplEndofwhowas:
		// useless whois delimiter

	case rplEndofinfo:
		// useless info delimiter

	case rplEndofhelp:
		// useless help delimiter

	case rplWhoiskeyvalue, rplKeyvalue, rplKeynotset, rplMetadataunsubok, rplMetadatasubs, rplMetadatasynclater:
		// useless metadata replies

	case rplStatscommands:
		var command, count string
		if err := msg.ParseParams(nil, &command, &count); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("The command %s was used %s times on the server", command, count),
		}, nil

	case rplUmodeis:
		if !s.receivedUserMode {
			// ignore the first RPL_UMODEIS on join
			s.receivedUserMode = true
			return nil, nil
		}
		return InfoEvent{
			Message: fmt.Sprintf("The current user modes are: %s", strings.Join(msg.Params[1:], " ")),
		}, nil

	case rplStatsuptime:
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("The server current uptime is: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplStatsconn, rplLuserclient:
		return InfoEvent{
			Prefix:  "Stats",
			Message: msg.Params[len(msg.Params)-1],
		}, nil

	case rplLuserop:
		var ops string
		if err := msg.ParseParams(nil, &ops); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("There are %s operators online", ops),
		}, nil

	case rplLuserunknown:
		var connections string
		if err := msg.ParseParams(nil, &connections); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("There are %s unknown user connections", connections),
		}, nil

	case rplLuserchannels:
		var channels string
		if err := msg.ParseParams(nil, &channels); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("There are %s channels on the server", channels),
		}, nil

	case rplLuserme:
		return InfoEvent{
			Prefix:  "Stats",
			Message: fmt.Sprintf("The server current stats are: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplAdminloc1:
		return InfoEvent{
			Prefix:  "Admin",
			Message: fmt.Sprintf("The server location/environment information is: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplAdminloc2:
		return InfoEvent{
			Prefix:  "Admin",
			Message: fmt.Sprintf("The server organization information is: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplAdminemail:
		return InfoEvent{
			Prefix:  "Admin",
			Message: fmt.Sprintf("The server email contact is: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplLocalusers:
		if len(msg.Params) >= 4 {
			var currentUsers, maxUsers string
			if err := msg.ParseParams(nil, &currentUsers, &maxUsers); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "Stats",
				Message: fmt.Sprintf("There are %s online users on this server, out of a maximum of %s users", currentUsers, maxUsers),
			}, nil
		} else {
			return InfoEvent{
				Prefix:  "Stats",
				Message: fmt.Sprintf("The server current local user counts are: %s", msg.Params[len(msg.Params)-1]),
			}, nil
		}

	case rplGlobalusers:
		if len(msg.Params) >= 4 {
			var currentUsers, maxUsers string
			if err := msg.ParseParams(nil, &currentUsers, &maxUsers); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "Stats",
				Message: fmt.Sprintf("There are %s online users on the network, out of a maximum of %s users", currentUsers, maxUsers),
			}, nil
		} else {
			return InfoEvent{
				Prefix:  "Stats",
				Message: fmt.Sprintf("The server current global user counts are: %s", msg.Params[len(msg.Params)-1]),
			}, nil
		}

	case rplWhoiscertfp:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s %s", nick, text),
		}, nil

	case rplUnaway:
		return InfoEvent{
			Message: "You are now marked as back from being away",
		}, nil

	case rplNowaway:
		return InfoEvent{
			Message: "You are now marked as away",
		}, nil

	case rplWhoisregnick:
		var nick string
		if err := msg.ParseParams(nil, &nick); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s has identified and is registered to the server", nick),
		}, nil

	case rplWhoisuser:
		var nick, username, host, realname string
		if err := msg.ParseParams(nil, &nick, &username, &host, nil, &realname); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s has username %s and host %s (mask %s!%s@%s); their realname is %s", nick, username, host, nick, username, host, realname),
		}, nil

	case rplWhoisserver:
		var nick, server, serverInfo string
		if err := msg.ParseParams(nil, &nick, &server, &serverInfo); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s is connected through the server %s (%s)", nick, server, serverInfo),
		}, nil

	case rplWhoisoperator:
		var nick, opertype string
		if err := msg.ParseParams(nil, &nick, &opertype); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s %s", nick, opertype),
		}, nil

	case rplWhowasuser:
		var nick, username, host, realname string
		if err := msg.ParseParams(nil, &nick, &username, &host, nil, &realname); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s was last seen with username %s and host %s (mask %s!%s@%s); their realname was %s", nick, username, host, nick, username, host, realname),
		}, nil

	case rplWhoisidle:
		var nick, idleText, signonText string
		if err := msg.ParseParams(nil, &nick, &idleText, &signonText); err != nil {
			return nil, err
		}
		idleSeconds, err := strconv.ParseInt(idleText, 10, 64)
		if err != nil {
			return nil, err
		}
		signon, err := strconv.ParseInt(signonText, 10, 64)
		if err != nil {
			return nil, err
		}
		idle := (time.Duration(idleSeconds) * time.Second).String()
		t := time.Unix(signon, 0)
		text := fmt.Sprintf("%s was idle for %s; they signed-on on %s", nick, idle, t.Local().Format("January 2 at 15:04"))
		return InfoEvent{
			Prefix:  "User",
			Message: text,
		}, nil

	case rplWhoischannels:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s has joined channels: %s", nick, text),
		}, nil

	case rplWhoisspecial:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s is also: %s", nick, text),
		}, nil

	case rplList:
		var channel, count, topic string
		if err := msg.ParseParams(nil, &channel, &count, &topic); err != nil {
			return nil, err
		}
		s.pendingList = append(s.pendingList, ListItem{
			Channel: channel,
			Count:   count,
			Topic:   topic,
		})
		return nil, nil

	case rplListend:
		list := s.pendingList
		s.pendingList = nil
		return list, nil

	case rplChannelmodeis:
		var channel string
		if err := msg.ParseParams(nil, &channel); err != nil {
			return nil, err
		}
		text := fmt.Sprintf("%s has modes %s", channel, strings.Join(msg.Params[2:], " "))
		return InfoEvent{
			Message: text,
		}, nil

	case rplCreationTime:
		var channel, creationTime string
		if err := msg.ParseParams(nil, &channel, &creationTime); err != nil {
			return nil, err
		}
		creation, err := strconv.ParseInt(creationTime, 10, 64)
		if err != nil {
			return nil, err
		}
		t := time.Unix(creation, 0)
		text := fmt.Sprintf("%s was created on %s", channel, t.Local().Format("January 2, 2006"))
		return InfoEvent{
			Message: text,
		}, nil

	case rplWhoisaccount:
		var nick, account string
		if err := msg.ParseParams(nil, &nick, &account); err != nil {
			return nil, err
		}
		if nick != account {
			return InfoEvent{
				Prefix:  "User",
				Message: fmt.Sprintf("%s is authenticated as %s", nick, account),
			}, nil
		} else {
			return InfoEvent{
				Prefix:  "User",
				Message: fmt.Sprintf("%s is authenticated", nick),
			}, nil
		}

	case rplInvitelist, rplInvexlist:
		if len(msg.Params) == 2 { // RPL_INVITELIST
			var channel string
			if err := msg.ParseParams(nil, &channel); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "Invite",
				Message: fmt.Sprintf("You were previously invited to the channel %s", channel),
			}, nil
		} else { // RPL_INVEXLIST
			var channel, mask string
			if err := msg.ParseParams(nil, &channel, &mask); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "Invite-free",
				Message: fmt.Sprintf("The channel %s can be joined without invites from host %s", channel, mask),
			}, nil
		}

	case rplWhoisactually:
		if len(msg.Params) == 3 {
			var nick, text string
			if err := msg.ParseParams(nil, &nick, &text); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "User",
				Message: fmt.Sprintf("%s %s", nick, text),
			}, nil
		} else if len(msg.Params) >= 4 {
			var nick string
			if err := msg.ParseParams(nil, &nick); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "User",
				Message: fmt.Sprintf("%s is actually using the host %s", nick, msg.Params[len(msg.Params)-2]),
			}, nil
		}

	case rplExceptlist:
		var channel, mask string
		if err := msg.ParseParams(nil, &channel, &mask); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Exempt",
			Message: fmt.Sprintf("The channel %s exempts from bans users from host %s", channel, mask),
		}, nil

	case rplVersion:
		var version string
		if err := msg.ParseParams(nil, &version); err != nil {
			return nil, err
		}
		return InfoEvent{
			Message: fmt.Sprintf("The server is running: %s", version),
		}, nil

	case rplLinks:
		var prefix, last string
		if err := msg.ParseParams(nil, &prefix, nil, &last); err != nil {
			return nil, err
		}
		hop, info, ok := strings.Cut(last, " ")
		if !ok {
			hop = "0"
			info = last
		}
		var count int
		if c, err := strconv.Atoi(hop); err == nil {
			count = c
		}
		if count < 0 || count > 255 {
			return nil, fmt.Errorf("invalid server hop count: %q", hop)
		}
		return InfoEvent{
			Prefix:  "Link",
			Message: fmt.Sprintf("The network has server %s%s (%s)", strings.Repeat("* ", count), prefix, info),
		}, nil

	case rplBanlist:
		if len(msg.Params) >= 5 {
			var channel, mask, who, whenText string
			if err := msg.ParseParams(nil, &channel, &mask, &who, &whenText); err != nil {
				return nil, err
			}
			when, err := strconv.ParseInt(whenText, 10, 64)
			if err != nil {
				return nil, err
			}
			t := time.Unix(when, 0).Local().Format("January 2 2006 at 15:04")
			return InfoEvent{
				Prefix:  "Ban",
				Message: fmt.Sprintf("The channel %s has %s banned by %s on %s", channel, mask, who, t),
			}, nil
		} else {
			var channel, mask string
			if err := msg.ParseParams(nil, &channel, &mask); err != nil {
				return nil, err
			}
			return InfoEvent{
				Prefix:  "Ban",
				Message: fmt.Sprintf("The channel %s has %s banned", channel, mask),
			}, nil
		}

	case rplInfo:
		var text string
		if err := msg.ParseParams(nil, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Info",
			Message: text,
		}, nil

	case rplMotd:
		return InfoEvent{
			Prefix:  "MotD",
			Message: msg.Params[1],
		}, nil

	case rplWhoishost:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s %s", nick, text),
		}, nil

	case rplWhoismodes:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s %s", nick, text),
		}, nil

	case rplYoureoper:
		var text string
		if err := msg.ParseParams(nil, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Message: text,
		}, nil

	case rplRehashing:
		return InfoEvent{
			Message: "The server configuration is now reloading (rehash)",
		}, nil

	case rplTime:
		return InfoEvent{
			Message: fmt.Sprintf("The server current local time is: %s", msg.Params[len(msg.Params)-1]),
		}, nil

	case rplWhoissecure:
		var nick, text string
		if err := msg.ParseParams(nil, &nick, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "User",
			Message: fmt.Sprintf("%s %s", nick, text),
		}, nil

	case rplMetadatasubok:
		if err := msg.ParseParams(nil); err != nil {
			return nil, err
		}
		for _, key := range msg.Params[1:] {
			s.metadataSubs[key] = struct{}{}
		}
		return nil, nil

	case rplHelpstart, rplHelptxt:
		var text string
		if err := msg.ParseParams(nil, nil, &text); err != nil {
			return nil, err
		}
		return InfoEvent{
			Prefix:  "Help",
			Message: text,
		}, nil

	default:
		if msg.IsReply() {
			if len(msg.Params) < 2 {
				return nil, msg.errNotEnoughParams(2)
			}
			if msg.Command == rplUmodeis && !s.receivedUserMode {
				// ignore the first RPL_UMODEIS on join
				s.receivedUserMode = true
				return nil, nil

			}
			if msg.Command == errUnknowncommand {
				switch msg.Params[1] {
				case "BOUNCER":
					// ignore any error in response to unconditional BOUNCER LISTNETWORKS
					return nil, nil
				case "METADATA":
					// ignore any error in response to unconditional METADATA SUB
					return nil, nil
				}
			}
			return ErrorEvent{
				Severity: ReplySeverity(msg.Command),
				Code:     msg.Command,
				Message:  strings.Join(msg.Params[1:], " "),
			}, nil
		}
	}
	return nil, nil
}
