package irc

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Session) handleChannelMessage(msg Message, playback bool) (Event, error) {
	switch msg.Command {
	case "JOIN":
		var channel string
		if err := msg.ParseParams(&channel); err != nil {
			return nil, err
		}

		if playback {
			return UserJoinEvent{
				User:    msg.Prefix.Name,
				Channel: channel,
				Time:    msg.TimeOrNow(),
			}, nil
		}

		nickCf := s.Casemap(msg.Prefix.Name)
		channelCf := s.Casemap(channel)

		if s.IsMe(nickCf) {
			s.channels[channelCf] = Channel{
				Name:    msg.Params[0],
				Members: map[*User]ChannelMember{},
			}
			if _, ok := s.enabledCaps["away-notify"]; ok {
				// Only try to know who is away if the list is
				// updated by the server via away-notify.
				// Otherwise, it'll become outdated over time.
				s.Who(channel)
			}
		} else if c, ok := s.channels[channelCf]; ok {
			if _, ok := s.users[nickCf]; !ok {
				s.users[nickCf] = &User{Name: msg.Prefix.Copy()}
			}
			c.Members[s.users[nickCf]] = ChannelMember{}
			return UserJoinEvent{
				User:    msg.Prefix.Name,
				Channel: c.Name,
				Time:    msg.TimeOrNow(),
			}, nil
		}

	case "PART":
		var channel string
		if err := msg.ParseParams(&channel); err != nil {
			return nil, err
		}

		if playback {
			return UserPartEvent{
				User:    msg.Prefix.Name,
				Channel: channel,
				Time:    msg.TimeOrNow(),
			}, nil
		}

		nickCf := s.Casemap(msg.Prefix.Name)
		channelCf := s.Casemap(channel)

		if s.IsMe(nickCf) {
			if c, ok := s.channels[channelCf]; ok {
				delete(s.channels, channelCf)
				for u := range c.Members {
					s.cleanUser(u)
				}
				return SelfPartEvent{
					Channel: c.Name,
				}, nil
			}
		} else if c, ok := s.channels[channelCf]; ok {
			if u, ok := s.users[nickCf]; ok {
				delete(c.Members, u)
				s.cleanUser(u)
				s.typings.Done(channelCf, nickCf)
				return UserPartEvent{
					User:    u.Name.Name,
					Channel: c.Name,
					Time:    msg.TimeOrNow(),
				}, nil
			}
		}

	case "KICK":
		var channel, nick string
		if err := msg.ParseParams(&channel, &nick); err != nil {
			return nil, err
		}

		if playback {
			return UserPartEvent{
				User:    nick,
				Channel: channel,
				Time:    msg.TimeOrNow(),
			}, nil
		}

		nickCf := s.Casemap(nick)
		channelCf := s.Casemap(channel)

		if s.IsMe(nickCf) {
			if c, ok := s.channels[channelCf]; ok {
				delete(s.channels, channelCf)
				for u := range c.Members {
					s.cleanUser(u)
				}
				return SelfPartEvent{
					Channel: c.Name,
				}, nil
			}
		} else if c, ok := s.channels[channelCf]; ok {
			if u, ok := s.users[nickCf]; ok {
				delete(c.Members, u)
				s.cleanUser(u)
				s.typings.Done(channelCf, nickCf)
				return UserPartEvent{
					User:    nick,
					Channel: c.Name,
					Time:    msg.TimeOrNow(),
				}, nil
			}
		}

	case "QUIT":
		if playback {
			return UserQuitEvent{
				User: msg.Prefix.Name,
				Time: msg.TimeOrNow(),
			}, nil
		}

		nickCf := s.Casemap(msg.Prefix.Name)

		if u, ok := s.users[nickCf]; ok {
			u.Disconnected = true
			var channels []string
			for channelCf, c := range s.channels {
				if _, ok := c.Members[u]; ok {
					channels = append(channels, c.Name)
					delete(c.Members, u)
					s.cleanUser(u)
					s.typings.Done(channelCf, nickCf)
				}
			}
			return UserQuitEvent{
				User:     u.Name.Name,
				Channels: channels,
				Time:     msg.TimeOrNow(),
			}, nil
		}

	case rplMononline:
		for _, target := range strings.Split(msg.Params[1], ",") {
			prefix := ParsePrefix(target)
			if prefix == nil {
				continue
			}
			nickCf := s.casemap(prefix.Name)

			if _, ok := s.monitors[nickCf]; ok {
				u, ok := s.users[nickCf]
				if !ok {
					u = &User{
						Name: prefix,
					}
					s.users[nickCf] = u
				}
				if u.Disconnected {
					u.Disconnected = false
					return UserOnlineEvent{
						User: u.Name.Name,
					}, nil
				}
			}
		}

	case rplMonoffline:
		for _, target := range strings.Split(msg.Params[1], ",") {
			prefix := ParsePrefix(target)
			if prefix == nil {
				continue
			}
			nickCf := s.casemap(prefix.Name)

			if _, ok := s.monitors[nickCf]; ok {
				u, ok := s.users[nickCf]
				if !ok {
					u = &User{
						Name: prefix,
					}
					s.users[nickCf] = u
				}
				if !u.Disconnected {
					u.Disconnected = true
					return UserOfflineEvent{
						User: u.Name.Name,
					}, nil
				}
			}
		}

	case rplNamreply:
		var channel, names string
		if err := msg.ParseParams(nil, nil, &channel, &names); err != nil {
			return nil, err
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok {

			for _, name := range ParseNameReply(names, s.prefixSymbols) {
				nickCf := s.Casemap(name.Name.Name)

				if _, ok := s.users[nickCf]; !ok {
					s.users[nickCf] = &User{Name: name.Name.Copy()}
				}
				m := c.Members[s.users[nickCf]]
				m.Membership = name.PowerLevel
				c.Members[s.users[nickCf]] = m
			}

			s.channels[channelCf] = c
		}

	case rplEndofnames:
		var channel string
		if err := msg.ParseParams(nil, &channel); err != nil {
			return nil, err
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok && !c.complete {
			c.complete = true
			s.channels[channelCf] = c
			ev := SelfJoinEvent{
				Channel: c.Name,
				Topic:   c.Topic,
				Read:    c.Read,
			}
			if stamp, ok := s.pendingChannels[channelCf]; ok && time.Since(stamp) < 5*time.Second {
				ev.Requested = true
			}
			return ev, nil
		}

	case rplTopic:
		var channel, topic string
		if err := msg.ParseParams(nil, &channel, &topic); err != nil {
			return nil, err
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok {
			c.Topic = topic
			s.channels[channelCf] = c
		}

	case rplTopicwhotime:
		var channel, topicWho, topicTime string
		if err := msg.ParseParams(nil, &channel, &topicWho, &topicTime); err != nil {
			return nil, err
		}

		channelCf := s.Casemap(channel)

		// ignore the error, we still have topicWho
		t, _ := strconv.ParseInt(topicTime, 10, 64)

		if c, ok := s.channels[channelCf]; ok {
			c.TopicWho = ParsePrefix(topicWho)
			c.TopicTime = time.Unix(t, 0)
			s.channels[channelCf] = c
		}

	case rplNotopic:
		var channel string
		if err := msg.ParseParams(nil, &channel); err != nil {
			return nil, err
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok {
			c.Topic = ""
			s.channels[channelCf] = c
		}

	case "TOPIC":
		var channel, topic string
		if err := msg.ParseParams(&channel, &topic); err != nil {
			return nil, err
		}

		if playback {
			return TopicChangeEvent{
				Channel: channel,
				Topic:   topic,
				Time:    msg.TimeOrNow(),
				Who:     msg.Prefix.Name,
			}, nil
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok {
			c.Topic = topic
			c.TopicWho = msg.Prefix.Copy()
			c.TopicTime = msg.TimeOrNow()
			s.channels[channelCf] = c
			return TopicChangeEvent{
				Channel: c.Name,
				Topic:   c.Topic,
				Time:    msg.TimeOrNow(),
				Who:     msg.Prefix.Name,
			}, nil
		}

	case "MODE":
		var channel string
		if err := msg.ParseParams(&channel, nil); err != nil {
			return nil, err
		}
		mode := strings.Join(msg.Params[1:], " ")

		if playback {
			return ModeChangeEvent{
				Channel: channel,
				Mode:    mode,
				Time:    msg.TimeOrNow(),
			}, nil
		}

		channelCf := s.Casemap(channel)

		if c, ok := s.channels[channelCf]; ok {
			modeChanges, err := ParseChannelMode(msg.Params[1], msg.Params[2:], s.chanmodes, s.prefixModes)
			if err != nil {
				return nil, err
			}
			for _, change := range modeChanges {
				i := strings.IndexByte(s.prefixModes, change.Mode)
				if i < 0 {
					continue
				}
				nickCf := s.Casemap(change.Param)
				user := s.users[nickCf]
				m, ok := c.Members[user]
				if !ok {
					continue
				}
				newMembership := []byte(m.Membership)
				if change.Enable {
					if strings.ContainsRune(m.Membership, rune(s.prefixSymbols[i])) {
						continue
					}
					newMembership = append(newMembership, s.prefixSymbols[i])
					sort.Slice(newMembership, func(i, j int) bool {
						i = strings.IndexByte(s.prefixSymbols, newMembership[i])
						j = strings.IndexByte(s.prefixSymbols, newMembership[j])
						return i < j
					})
				} else if j := strings.IndexByte(m.Membership, s.prefixSymbols[i]); j >= 0 {
					newMembership = append(newMembership[:j], newMembership[j+1:]...)
				}
				m.Membership = string(newMembership)
				c.Members[user] = m
			}
			s.channels[channelCf] = c
			return ModeChangeEvent{
				Channel: c.Name,
				Mode:    mode,
				Time:    msg.TimeOrNow(),
			}, nil
		}

	case "INVITE":
		var nick, channel string
		if err := msg.ParseParams(&nick, &channel); err != nil {
			return nil, err
		}

		return InviteEvent{
			Inviter: msg.Prefix.Name,
			Invitee: nick,
			Channel: channel,
		}, nil

	case rplInviting:
		var nick, channel string
		if err := msg.ParseParams(nil, &nick, &channel); err != nil {
			return nil, err
		}

		return InviteEvent{
			Inviter: s.nick,
			Invitee: nick,
			Channel: channel,
		}, nil

	case "AWAY":
		nickCf := s.Casemap(msg.Prefix.Name)

		if u, ok := s.users[nickCf]; ok {
			u.Away = len(msg.Params) == 1
		}

	case "PRIVMSG", "NOTICE":
		if !s.registered && msg.Command == "NOTICE" {
			return nil, nil
		}

		var target string
		if err := msg.ParseParams(&target); err != nil {
			return nil, err
		}

		targetCf := strings.TrimLeft(s.casemap(target), s.statusmsg)
		nickCf := s.casemap(msg.Prefix.Name)
		if !playback {
			s.typings.Done(targetCf, nickCf)
		}
		ev, err := s.newMessageEvent(msg)
		if err != nil {
			return nil, err
		}
		if c, ok := s.channels[targetCf]; ok {
			if u, ok := s.users[nickCf]; ok {
				if m, ok := c.Members[u]; ok {
					if ev.Time.After(m.LastActive) {
						m.LastActive = ev.Time
						c.Members[u] = m
					}
				}
			}
		}
		return ev, nil

	case "TAGMSG":
		if playback {
			return nil, nil
		}

		var target string
		if err := msg.ParseParams(&target); err != nil {
			return nil, err
		}

		targetCf := s.casemap(target)
		nickCf := s.casemap(msg.Prefix.Name)

		if s.IsMe(msg.Prefix.Name) {
			// TAGMSG from self
			break
		}

		if t, ok := msg.Tags["+typing"]; ok {
			switch t {
			case "active":
				s.typings.Active(targetCf, nickCf)
			case "paused", "done":
				s.typings.Done(targetCf, nickCf)
			}
		}

	case "NICK":
		var nick string
		if err := msg.ParseParams(&nick); err != nil {
			return nil, err
		}

		if playback {
			return UserNickEvent{
				User:       nick,
				FormerNick: msg.Prefix.Name,
				Time:       msg.TimeOrNow(),
			}, nil
		}

		nickCf := s.Casemap(msg.Prefix.Name)
		newNick := nick
		newNickCf := s.Casemap(newNick)

		if formerUser, ok := s.users[nickCf]; ok {
			formerUser.Name.Name = newNick
			delete(s.users, nickCf)
			s.users[newNickCf] = formerUser
		} else {
			break
		}

		if s.IsMe(msg.Prefix.Name) {
			s.nick = newNick
			s.nickCf = newNickCf
			return SelfNickEvent{
				FormerNick: msg.Prefix.Name,
			}, nil
		} else {
			return UserNickEvent{
				User:       nick,
				FormerNick: msg.Prefix.Name,
				Time:       msg.TimeOrNow(),
			}, nil
		}
	}
	return nil, nil
}

func (s *Session) newMessageEvent(msg Message) (ev MessageEvent, err error) {
	var rawTarget, content string
	if err := msg.ParseParams(&rawTarget, &content); err != nil {
		return ev, err
	}

	i := strings.IndexFunc(rawTarget, func(r rune) bool {
		return !strings.ContainsRune(s.statusmsg, r)
	})
	if i < 0 {
		i = len(rawTarget)
	}
	prefix := rawTarget[:i]
	target := rawTarget[i:]

	ev = MessageEvent{
		User:         msg.Prefix.Name, // TODO correctly casemap
		Target:       target,          // TODO correctly casemap
		TargetPrefix: prefix,
		Command:      msg.Command,
		Content:      content,
		Time:         msg.TimeOrNow(),
	}

	if s.IsMe(target) {
		if context := msg.Tags["+draft/channel-context"]; context != "" {
			target = context
			ev.TargetPrefix = "*"
		}
	}
	targetCf := s.Casemap(target)
	if c, ok := s.channels[targetCf]; ok {
		ev.Target = c.Name
		ev.TargetIsChannel = true
	}

	return ev, nil
}

func (s *Session) cleanUser(parted *User) {
	nameCf := s.Casemap(parted.Name.Name)
	if _, ok := s.monitors[nameCf]; ok {
		return
	}
	for _, c := range s.channels {
		if _, ok := c.Members[parted]; ok {
			return
		}
	}
	delete(s.users, nameCf)
}
