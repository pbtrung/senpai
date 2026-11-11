package irc

import (
	"sort"
	"strings"
	"time"
)

// Values taken by the "@+typing=" client tag.  TypingUnspec means the value or
// tag is absent.
const (
	TypingUnspec = iota
	TypingActive
	TypingPaused
	TypingDone
)

// User is a known IRC user.
type User struct {
	Name         *Prefix // the nick, user and hostname of the user if known.
	Away         bool    // whether the user is away or not
	Disconnected bool    // can only be true for monitored users.
}

type ChannelMember struct {
	Membership string
	LastActive time.Time
}

// Channel is a joined channel.
type Channel struct {
	Name      string                  // the name of the channel.
	Members   map[*User]ChannelMember // the set of members associated with their membership.
	Topic     string                  // the topic of the channel, or "" if absent.
	TopicWho  *Prefix                 // the name of the last user who set the topic.
	TopicTime time.Time               // the last time the topic has been changed.
	Read      time.Time               // the time until which messages were read.

	complete bool // whether this structure is fully initialized.
}

type Metadata struct {
	Pinned bool
	Muted  bool
}

// SessionParams defines how to connect to an IRC server.
type SessionParams struct {
	Nickname string
	Username string
	RealName string
	NetID    string
	Auth     SASLClient
}

type Session struct {
	out              chan<- Message
	virtual          bool
	parent           *Session
	multilineID      uint64
	multilineBatches map[string]*multilineBatch
	closed           bool
	registered       bool
	typings          *Typings               // incoming typing notifications.
	typingStamps     map[string]typingStamp // user typing instants.

	nick     string
	nickCf   string // casemapped nickname.
	user     string
	real     string
	acct     string
	host     string
	netID    string
	netAttrs map[string]string
	auth     SASLClient

	availableCaps map[string]string
	enabledCaps   map[string]struct{}
	metadataSubs  map[string]struct{}

	serverName    string
	defaultPrefix *Prefix

	// ISUPPORT features
	casemap       func(string) string
	chanmodes     [4]string
	chantypes     string
	linelen       int
	historyLimit  int
	prefixSymbols string
	prefixModes   string
	statusmsg     string
	monitor       bool
	whox          bool
	listMask      bool
	upload        string

	clientTagListIsAllow bool // whether clientTagList is an allowlist (true) or a blocklist (false)
	clientTagList        map[string]struct{}

	users                  map[string]*User        // known users.
	channels               map[string]Channel      // joined channels.
	metadata               map[string]Metadata     // known target metadata.
	chBatches              map[string]HistoryEvent // channel history batches being processed.
	chReqs                 map[string]struct{}     // set of targets for which history is currently requested.
	targetsBatchID         string                  // ID of the channel history targets batch being processed.
	targetsBatch           HistoryTargetsEvent     // channel history targets batch being processed.
	searchBatchID          string                  // ID of the search targets batch being processed.
	searchBatch            SearchEvent             // search batch being processed.
	bouncerNetworksBatchID string                  // ID of the bouncer network batch being processed.
	bouncerNetworksBatch   BouncerNetworkListEvent // bouncer network batch being processed.
	monitors               map[string]struct{}     // set of users we want to monitor (and keep even if they are disconnected).
	pendingList            ListEvent               // current list response being received (flushed on list end).

	pendingChannels map[string]time.Time // set of join requests stamps for channels.

	receivedISupport bool
	receivedUserMode bool
}

func newSession(out chan<- Message, params SessionParams) *Session {
	s := &Session{
		out:             out,
		typings:         NewTypings(),
		typingStamps:    map[string]typingStamp{},
		nick:            params.Nickname,
		nickCf:          CasemapASCII(params.Nickname),
		user:            params.Username,
		real:            params.RealName,
		netID:           params.NetID,
		auth:            params.Auth,
		availableCaps:   map[string]string{},
		enabledCaps:     map[string]struct{}{},
		metadataSubs:    map[string]struct{}{},
		casemap:         CasemapRFC1459,
		chantypes:       "#&",
		linelen:         512,
		historyLimit:    100,
		prefixSymbols:   "@+",
		prefixModes:     "ov",
		clientTagList:   map[string]struct{}{},
		users:           map[string]*User{},
		channels:        map[string]Channel{},
		metadata:        map[string]Metadata{},
		chBatches:       map[string]HistoryEvent{},
		chReqs:          map[string]struct{}{},
		monitors:        map[string]struct{}{},
		pendingChannels: map[string]time.Time{},
	}

	s.multilineBatches = make(map[string]*multilineBatch)
	return s
}

func NewSession(out chan<- Message, params SessionParams) *Session {
	s := newSession(out, params)
	s.send(NewMessage("CAP", "LS", "302"))
	for capability, immediate := range SupportedCapabilities {
		if capability == "soju.im/multiplex" && s.netID != "" {
			continue
		}
		if immediate || s.netID != "" {
			s.send(NewMessage("CAP", "REQ", capability))
		}
	}
	s.send(NewMessage("NICK", s.nick))
	s.send(NewMessage("USER", s.user, "0", "*", s.real))
	if s.auth != nil && s.auth.Early() {
		h := s.auth.Handshake()
		s.send(NewMessage("AUTHENTICATE", h))
		res, err := s.auth.Respond("+")
		if err != nil {
			s.send(NewMessage("AUTHENTICATE", "*"))
		} else {
			s.send(NewMessage("AUTHENTICATE", res))
		}
		s.auth = nil
	}

	if s.auth == nil {
		s.endRegistration()
	}

	return s
}

func (s *Session) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.typings.Close()
	if !s.virtual {
		close(s.out)
	}
}

// HasCapability reports whether the given capability has been negotiated
// successfully.
func (s *Session) HasCapability(capability string) bool {
	_, ok := s.enabledCaps[capability]
	return ok
}

func (s *Session) CanSendTag(clientTagWithoutThePlus string) bool {
	_, ok := s.clientTagList[clientTagWithoutThePlus]
	return ok == s.clientTagListIsAllow
}

func (s *Session) IsBouncer() bool {
	if s.HasCapability("soju.im/bouncer-networks") { // soju-compatible
		return true
	}
	if s.defaultPrefix != nil && s.defaultPrefix.Name == "irc.znc.in" { // ZNC
		return true
	}
	return false
}

// BouncerService returns the optional nick of the bouncer service user.
func (s *Session) BouncerService() string {
	switch s.serverName {
	case "soju":
		return "BouncerServ"
	}
	return ""
}

// UploadURL returns the URL to which files can be uploaded according to the FILEHOST specification.
func (s *Session) UploadURL() string {
	return s.upload
}

func (s *Session) HasListMask() bool {
	return s.listMask
}

func (s *Session) Nick() string {
	return s.nick
}

func (s *Session) NetID() string {
	return s.netID
}

// NickCf is our casemapped nickname.
func (s *Session) NickCf() string {
	return s.nickCf
}

func (s *Session) IsMe(nick string) bool {
	return s.nickCf == s.casemap(nick)
}

func (s *Session) IsChannel(name string) bool {
	return strings.IndexAny(name, s.chantypes) == 0
}

// Joined reports whether we are currently joined to a channel.
func (s *Session) Joined(channel string) bool {
	_, ok := s.channels[s.Casemap(channel)]
	return ok
}

func (s *Session) Casemap(name string) string {
	return s.casemap(name)
}

// Users returns the list of all known nicknames.
func (s *Session) Users() []string {
	users := make([]string, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u.Name.Name)
	}
	return users
}

// Names returns the list of users in the given target, or nil if the target
// is not a known channel or nick in the session.
// The list is sorted according to member name.
func (s *Session) Names(target string) []Member {
	var names []Member
	if s.IsChannel(target) {
		if c, ok := s.channels[s.Casemap(target)]; ok {
			names = make([]Member, 0, len(c.Members))
			for u, m := range c.Members {
				names = append(names, Member{
					PowerLevel:   m.Membership,
					Name:         u.Name.Copy(),
					Away:         u.Away,
					Disconnected: u.Disconnected,
					Self:         s.nickCf == s.casemap(u.Name.Name),
					LastActive:   m.LastActive,
				})
			}
		}
	} else if u, ok := s.users[s.Casemap(target)]; ok {
		names = append(names, Member{
			Name:         u.Name.Copy(),
			Away:         u.Away,
			Disconnected: u.Disconnected,
		})
		names = append(names, Member{
			Name: &Prefix{
				Name: s.nick,
			},
			Self: true,
		})
	}
	sort.Sort(members{
		m:        names,
		prefixes: s.prefixSymbols,
	})
	return names
}

// Typings returns the list of nickname who are currently typing.
func (s *Session) Typings(target string) []string {
	targetCf := s.casemap(target)
	res := s.typings.List(targetCf)
	for i := 0; i < len(res); i++ {
		if s.IsMe(res[i]) {
			res = append(res[:i], res[i+1:]...)
			i--
		} else if u, ok := s.users[res[i]]; ok {
			res[i] = u.Name.Name
		}
	}
	sort.Strings(res)
	return res
}

func (s *Session) TypingStops() <-chan Typing {
	return s.typings.Stops()
}

func (s *Session) ChannelsSharedWith(name string) []string {
	var user *User
	if u, ok := s.users[s.Casemap(name)]; ok && !u.Disconnected {
		user = u
	} else {
		return nil
	}
	var channels []string
	for _, c := range s.channels {
		if _, ok := c.Members[user]; ok {
			channels = append(channels, c.Name)
		}
	}
	return channels
}

func (s *Session) Topic(channel string) (topic string, who *Prefix, at time.Time) {
	channelCf := s.Casemap(channel)
	if c, ok := s.channels[channelCf]; ok {
		topic = c.Topic
		who = c.TopicWho
		at = c.TopicTime
	}
	return
}

func (s *Session) HandleMessage(msg Message) (Event, error) {
	if msg.Prefix == nil {
		if s.defaultPrefix != nil {
			msg.Prefix = s.defaultPrefix
		} else {
			msg.Prefix = &Prefix{
				Name: "*",
			}
		}
	}
	if s.registered {
		return s.handleRegistered(msg)
	} else {
		return s.handleUnregistered(msg)
	}
}

func (s *Session) handleMessageRegistered(msg Message, playback bool) (Event, error) {
	// Every numeric reply has a recipient and reply data. Validate before
	// dispatching to handlers which index those fields directly.
	if msg.IsReply() && len(msg.Params) < 2 {
		return nil, msg.errNotEnoughParams(2)
	}
	switch msg.Command {
	case "AUTHENTICATE", rplLoggedin, errNicklocked, errSaslfail, errSasltoolong, errSaslaborted, errSaslalready, rplSaslmechs, rplWelcome, rplMyinfo, rplIsupport, rplWhoreply, rplWhospecialreply, rplEndofwho, "CAP":
		return s.handleRegistrationMessage(msg)
	case "JOIN", "PART", "KICK", "QUIT", rplMononline, rplMonoffline, rplNamreply, rplEndofnames, rplTopic, rplTopicwhotime, rplNotopic, "TOPIC", "MODE", "INVITE", rplInviting, "AWAY", "PRIVMSG", "NOTICE", "TAGMSG", "NICK":
		return s.handleChannelMessage(msg, playback)
	case "BATCH":
		return s.handleBatchMessage(msg)
	case "MARKREAD", "METADATA", "BOUNCER":
		return s.handleMetadataMessage(msg)
	case "PING":
		var payload string
		if err := msg.ParseParams(&payload); err != nil {
			return nil, err
		}

		s.send(NewMessage("PONG", payload))

	case "ERROR":
		s.Close()

	case "FAIL", "WARN", "NOTE":
		if msg.Command == "FAIL" && len(msg.Params) > 2 && msg.Params[0] == "CHATHISTORY" {
			if msg.Params[2] == "TARGETS" {
				delete(s.chReqs, "")
			} else if len(msg.Params) > 3 {
				delete(s.chReqs, s.Casemap(msg.Params[3]))
			}
		}
		var severity Severity
		var code string
		if err := msg.ParseParams(nil, &code); err != nil {
			return nil, err
		}

		switch code {
		case "KEY_INVALID": // METADATA SUB failed: ignore
			return nil, nil
		}

		switch msg.Command {
		case "FAIL":
			severity = SeverityFail
		case "WARN":
			severity = SeverityWarn
		case "NOTE":
			severity = SeverityNote
		}

		return ErrorEvent{
			Severity: severity,
			Code:     code,
			Message:  strings.Join(msg.Params[2:], " "),
		}, nil
	default:
		return s.handleReplyMessage(msg)
	}
	return nil, nil
}
