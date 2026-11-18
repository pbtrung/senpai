package irc

import (
	"time"

	"golang.org/x/time/rate"
)

func (s *Session) SendRaw(raw string) {
	msg, err := ParseMessage(raw)
	if err != nil {
		return
	}
	s.send(msg)
}

func (s *Session) Send(command string, params ...string) {
	s.send(NewMessage(command, params...))
}

func (s *Session) List(pattern string) {
	if pattern != "" {
		s.send(NewMessage("LIST", pattern))
	} else {
		s.send(NewMessage("LIST"))
	}
}

func (s *Session) Join(channel, key string) {
	channelCf := s.Casemap(channel)
	s.pendingChannels[channelCf] = time.Now()
	if key == "" {
		s.send(NewMessage("JOIN", channel))
	} else {
		s.send(NewMessage("JOIN", channel, key))
	}
}

func (s *Session) Part(channel, reason string) {
	s.send(NewMessage("PART", channel, reason))
}

func (s *Session) ChangeTopic(channel, topic string) {
	s.send(NewMessage("TOPIC", channel, topic))
}

func (s *Session) Quit(reason string) {
	s.send(NewMessage("QUIT", reason))
}

func (s *Session) ChangeNick(nick string) {
	s.send(NewMessage("NICK", nick))
}

func (s *Session) Who(target string) {
	if s.whox {
		// only request what we need, to optimize server who cache hits and reduce traffic
		s.send(NewMessage("WHO", target, "%uhnf"))
	} else {
		s.send(NewMessage("WHO", target))
	}
}

func (s *Session) ChangeMode(channel, flags string, args []string) {
	if flags != "" {
		args = append([]string{channel, flags}, args...)
	} else {
		args = append([]string{channel}, args...)
	}
	s.send(NewMessage("MODE", args...))
}

func (s *Session) Search(target, text string) {
	if _, ok := s.enabledCaps["soju.im/search"]; !ok {
		return
	}
	attrs := make(map[string]string)
	attrs["text"] = text
	if target != "" {
		attrs["in"] = target
	}
	s.send(NewMessage("SEARCH", formatTags(attrs)))
}

func (s *Session) Away(message string) {
	if message != "" {
		s.send(NewMessage("AWAY", message))
	} else {
		s.send(NewMessage("AWAY"))
	}
}

func (s *Session) PrivMsg(target, content string) {
	hostLen := len(s.host)
	if hostLen == 0 {
		hostLen = len("255.255.255.255")
	}
	maxMessageLen := s.linelen -
		len(":!@ PRIVMSG  :\r\n") -
		len(s.nick) -
		len(s.user) -
		hostLen -
		len(target)
	s.sendText("PRIVMSG", target, content, maxMessageLen)
	targetCf := s.Casemap(target)
	delete(s.typingStamps, targetCf)
}

func (s *Session) Typing(target string) {
	if !s.HasCapability("message-tags") || !s.CanSendTag("typing") {
		return
	}
	targetCf := s.casemap(target)
	now := time.Now()
	t, ok := s.typingStamps[targetCf]
	if ok && ((t.Type == TypingActive && now.Sub(t.Last).Seconds() < 3.0) || !t.Limit.Allow()) {
		return
	}
	if !ok {
		t.Limit = rate.NewLimiter(rate.Limit(1.0/3.0), 5)
		t.Limit.Reserve() // will always be OK
	}
	s.typingStamps[targetCf] = typingStamp{
		Last:  now,
		Type:  TypingActive,
		Limit: t.Limit,
	}
	s.send(NewMessage("TAGMSG", target).WithTag("+typing", "active"))
}

func (s *Session) TypingStop(target string) {
	if !s.HasCapability("message-tags") || !s.CanSendTag("typing") {
		return
	}
	targetCf := s.casemap(target)
	now := time.Now()
	t, ok := s.typingStamps[targetCf]
	if ok && (t.Type == TypingDone || !t.Limit.Allow()) {
		// don't send a +typing=done again if the last typing we sent was a +typing=done
		return
	}
	if !ok {
		t.Limit = rate.NewLimiter(rate.Limit(1), 5)
		t.Limit.Reserve() // will always be OK
	}
	s.typingStamps[targetCf] = typingStamp{
		Last:  now,
		Type:  TypingDone,
		Limit: t.Limit,
	}
	s.send(NewMessage("TAGMSG", target).WithTag("+typing", "done"))
}

func (s *Session) ReadGet(target string) {
	if _, ok := s.enabledCaps["draft/read-marker"]; ok {
		s.send(NewMessage("MARKREAD", target))
	}
}

func (s *Session) ReadSet(target string, timestamp time.Time) {
	if _, ok := s.enabledCaps["draft/read-marker"]; ok {
		s.send(NewMessage("MARKREAD", target, formatTimestamp(timestamp)))
	}
}

func (s *Session) MutedGet(target string) bool {
	return s.metadata[s.Casemap(target)].Muted
}

func (s *Session) MutedSet(target string, muted bool) (ok bool) {
	var v string
	if muted {
		v = "1"
	} else {
		v = "0"
	}
	k := "soju.im/muted"
	if _, ok = s.metadataSubs[k]; ok {
		s.send(NewMessage("METADATA", target, "SET", k, v))
	}
	return
}

func (s *Session) PinnedGet(target string) bool {
	return s.metadata[s.Casemap(target)].Pinned
}

func (s *Session) PinnedSet(target string, pinned bool) (ok bool) {
	var v string
	if pinned {
		v = "1"
	} else {
		v = "0"
	}
	k := "soju.im/pinned"
	if _, ok = s.metadataSubs[k]; ok {
		s.send(NewMessage("METADATA", target, "SET", "soju.im/pinned", v))
	}
	return
}

func (s *Session) MonitorAdd(target string) {
	targetCf := s.casemap(target)
	if _, ok := s.monitors[targetCf]; !ok {
		s.monitors[targetCf] = struct{}{}
		if s.monitor {
			s.send(NewMessage("MONITOR", "+", target))
		}
	}
}

func (s *Session) MonitorRemove(target string) {
	targetCf := s.casemap(target)
	if _, ok := s.monitors[targetCf]; ok {
		delete(s.monitors, targetCf)
		if s.monitor {
			s.send(NewMessage("MONITOR", "-", target))
		}
	}
}

func (s *Session) Whois(nick string) {
	s.send(NewMessage("WHOIS", nick))
}

func (s *Session) Whowas(nick string) {
	s.send(NewMessage("WHOWAS", nick))
}

func (s *Session) Invite(nick, channel string) {
	s.send(NewMessage("INVITE", nick, channel))
}

func (s *Session) Kick(nick, channel, comment string) {
	if comment == "" {
		s.send(NewMessage("KICK", channel, nick))
	} else {
		s.send(NewMessage("KICK", channel, nick, comment))
	}
}
