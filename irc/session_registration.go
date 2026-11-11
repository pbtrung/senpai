package irc

import (
	"fmt"
	"strings"
)

// SupportedCapabilities is the set of capabilities supported by this library.
// Value is false if the cap is deferred (to work around some daemons agfressive rate pre-conn-reg backlog limiting)
var SupportedCapabilities = map[string]bool{
	"away-notify":      false,
	"batch":            true,
	"cap-notify":       true,
	"echo-message":     true,
	"extended-monitor": false,
	"invite-notify":    false,
	"labeled-response": true,
	"message-tags":     true,
	"multi-prefix":     true,
	"sasl":             true,
	"server-time":      true,
	"setname":          false,
	"standard-replies": true,

	"draft/chathistory":               true,
	"draft/event-playback":            true,
	"draft/multiline":                 true,
	"soju.im/multiplex":               true,
	"soju.im/unread":                  true,
	"draft/metadata-2":                true,
	"draft/read-marker":               true,
	"soju.im/bouncer-networks-notify": true,
	"soju.im/bouncer-networks":        true,
	"soju.im/search":                  false,
}

func (s *Session) handleUnregistered(msg Message) (Event, error) {
	switch msg.Command {
	case errNicknameinuse:
		var nick string
		if err := msg.ParseParams(nil, &nick); err != nil {
			return nil, err
		}

		s.send(NewMessage("NICK", nick+"_"))
	case rplSaslsuccess:
		if s.auth != nil {
			s.endRegistration()
		}
	default:
		return s.handleRegistered(msg)
	}
	return nil, nil
}

func (s *Session) handleRegistrationMessage(msg Message) (Event, error) {
	switch msg.Command {
	case "AUTHENTICATE":
		if s.auth == nil {
			break
		}

		var payload string
		if err := msg.ParseParams(&payload); err != nil {
			return nil, err
		}

		res, err := s.auth.Respond(payload)
		if err != nil {
			s.send(NewMessage("AUTHENTICATE", "*"))
		} else {
			s.send(NewMessage("AUTHENTICATE", res))
		}

	case rplLoggedin:
		var nuh string
		if err := msg.ParseParams(nil, &nuh, &s.acct); err != nil {
			return nil, err
		}

		prefix := ParsePrefix(nuh)
		s.user = prefix.User
		s.host = prefix.Host

	case errNicklocked, errSaslfail, errSasltoolong, errSaslaborted, errSaslalready, rplSaslmechs:
		if s.auth != nil {
			s.endRegistration()
		}
		return ErrorEvent{
			Severity: SeverityFail,
			Code:     msg.Command,
			Message:  fmt.Sprintf("Registration failed: %s", strings.Join(msg.Params[1:], " ")),
		}, nil

	case rplWelcome:
		s.defaultPrefix = msg.Prefix
		if err := msg.ParseParams(&s.nick); err != nil {
			return nil, err
		}

		s.nickCf = s.Casemap(s.nick)
		s.registered = true
		s.users[s.nickCf] = &User{Name: &Prefix{
			Name: s.nick, User: s.user, Host: s.host,
		}}
		if s.host == "" {
			s.Who(s.nick)
		}

	case rplMyinfo:
		if err := msg.ParseParams(nil, nil, &s.serverName); err != nil {
			return nil, err
		}

	case rplIsupport:
		if len(msg.Params) < 3 {
			return nil, msg.errNotEnoughParams(3)
		}
		s.updateFeatures(msg.Params[1 : len(msg.Params)-1])
		if !s.receivedISupport {
			// notify only on first RPL_ISUPPORT
			s.receivedISupport = true
			return RegisteredEvent{}, nil
		}
		return nil, nil

	case rplWhoreply, rplWhospecialreply:
		var nick, host, flags, username string
		var err error
		if msg.Command == rplWhoreply {
			err = msg.ParseParams(nil, nil, &username, &host, nil, &nick, &flags, nil)
		} else {
			// we always request WHOX with %uhnf
			err = msg.ParseParams(nil, &username, &host, &nick, &flags)
		}
		if err != nil {
			return nil, err
		}

		nickCf := s.Casemap(nick)
		away := strings.ContainsRune(flags, 'G')

		if s.nickCf == nickCf {
			s.user = username
			s.host = host
		}

		if u, ok := s.users[nickCf]; ok {
			u.Away = away
		}

	case rplEndofwho:
		// do nothing

	case "CAP":
		var subcommand, caps string
		if err := msg.ParseParams(nil, &subcommand); err != nil {
			return nil, err
		}
		if len(msg.Params) > 3 && msg.Params[2] == "*" {
			if err := msg.ParseParams(nil, nil, nil, &caps); err != nil {
				return nil, err
			}
		} else {
			if err := msg.ParseParams(nil, nil, &caps); err != nil {
				return nil, err
			}
		}

		switch subcommand {
		case "ACK":
			for _, c := range ParseCaps(caps) {
				if c.Enable {
					s.enabledCaps[c.Name] = struct{}{}
				} else {
					delete(s.enabledCaps, c.Name)
				}

				if s.auth != nil && c.Name == "sasl" {
					h := s.auth.Handshake()
					s.send(NewMessage("AUTHENTICATE", h))
				} else if len(s.channels) != 0 && c.Name == "multi-prefix" {
					// TODO merge NAMES commands
					for channel := range s.channels {
						s.send(NewMessage("NAMES", channel))
					}
				} else if c.Name == "labeled-response" {
					if c.Enable {
						s.send(Message{
							Command: labelEnableCommand,
						})
					} else {
						s.send(Message{
							Command: labelDisableCommand,
						})
					}
				}
			}
		case "NAK":
			// do nothing
		case "LS", "NEW":
			var reqs []string
			for _, c := range ParseCaps(caps) {
				s.availableCaps[c.Name] = c.Value
				immediate, ok := SupportedCapabilities[c.Name]
				if c.Name == "soju.im/multiplex" && s.netID != "" {
					continue
				}
				if !ok {
					continue
				}
				if subcommand == "LS" && (immediate || s.netID != "") {
					// Already sent CAP, ignore
					continue
				}
				if _, ok := s.enabledCaps[c.Name]; ok {
					continue
				}
				reqs = append(reqs, c.Name)
			}
			if len(reqs) > 0 {
				s.send(NewMessage("CAP", "REQ", strings.Join(reqs, " ")))
			}
		case "DEL":
			for _, c := range ParseCaps(caps) {
				delete(s.availableCaps, c.Name)
				delete(s.enabledCaps, c.Name)
				if c.Name == "labeled-response" {
					s.send(Message{
						Command: labelDisableCommand,
					})
				}
			}
		}
	}
	return nil, nil
}

func (s *Session) endRegistration() {
	if s.registered {
		return
	}
	if len(s.enabledCaps) == 0 || s.HasCapability("draft/metadata-2") {
		// Best effort to avoid a round trip: subscribe to metadata if explicitly supported or if CAPs are not yet known
		s.send(NewMessage("METADATA", "*", "SUB", "soju.im/pinned", "soju.im/muted"))
	}
	if s.netID != "" {
		s.send(NewMessage("BOUNCER", "BIND", s.netID))
		s.send(NewMessage("CAP", "END"))
	} else {
		s.send(NewMessage("CAP", "END"))
		s.send(NewMessage("BOUNCER", "LISTNETWORKS"))
	}
}
