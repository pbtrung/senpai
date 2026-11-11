package irc

import (
	"strconv"
	"strings"
	"unicode"
)

func (s *Session) updateFeatures(features []string) {
	for _, f := range features {
		if f == "" || f == "-" || f == "=" || f == "-=" {
			continue
		}

		var (
			add   bool
			key   string
			value string
		)

		if strings.HasPrefix(f, "-") {
			add = false
			f = f[1:]
		} else {
			add = true
		}

		kv := strings.SplitN(f, "=", 2)
		key = strings.ToUpper(kv[0])
		if len(kv) > 1 {
			value = kv[1]
		}

		if !add {
			switch key {
			case "CLIENTTAGDENY":
				clear(s.clientTagList)
				s.clientTagListIsAllow = false
			case "CHANMODES":
				s.chanmodes = [4]string{}
			case "CHANTYPES":
				s.chantypes = "#&"
			case "CHATHISTORY":
				s.historyLimit = 100
			case "ELIST":
				s.listMask = false
			case "LINELEN":
				s.linelen = 512
			case "MONITOR":
				s.monitor = false
			case "PREFIX":
				s.prefixModes = "ov"
				s.prefixSymbols = "@+"
			case "STATUSMSG":
				s.statusmsg = ""
			case "WHOX":
				s.whox = false
			case "SOJU.IM/FILEHOST":
				s.upload = ""
			}
			continue
		}

	Switch:
		switch key {
		case "BOUNCER_NETID":
			s.netID = value
		case "CASEMAPPING":
			oldNickCf := s.nickCf
			switch value {
			case "ascii":
				s.casemap = CasemapASCII
			default:
				s.casemap = CasemapRFC1459
			}
			s.nickCf = s.casemap(s.nick)
			if oldNickCf != s.nickCf {
				if u, ok := s.users[oldNickCf]; ok {
					delete(s.users, oldNickCf)
					s.users[s.nickCf] = u
				}
			}
		case "CLIENTTAGDENY":
			clear(s.clientTagList)
			s.clientTagListIsAllow = strings.HasPrefix(value, "*")
			tags := strings.Split(value, ",")
			if s.clientTagListIsAllow {
				tags = tags[1:]
			}
			for _, tag := range tags {
				if s.clientTagListIsAllow {
					if !strings.HasPrefix(tag, "-") {
						continue
					}
					tag = tag[1:]
				} else if strings.HasPrefix(tag, "-") {
					continue
				}
				s.clientTagList[tag] = struct{}{}
			}
		case "CHANMODES":
			s.chanmodes = [4]string{}
			// We only care about the first four params
			types := strings.SplitN(value, ",", 5)
			for i := 0; i < len(types) && i < len(s.chanmodes); i++ {
				s.chanmodes[i] = types[i]
			}
		case "CHANTYPES":
			s.chantypes = value
		case "CHATHISTORY":
			historyLimit, err := strconv.Atoi(value)
			if err == nil && historyLimit > 0 {
				s.historyLimit = historyLimit
			}
		case "ELIST":
			s.listMask = strings.Contains(strings.ToUpper(value), "M")
		case "LINELEN":
			linelen, err := strconv.Atoi(value)
			if err == nil && linelen >= 512 && linelen <= 65536 {
				s.linelen = linelen
			}
		case "MONITOR":
			monitor, err := strconv.Atoi(value)
			s.monitor = value == "" || (err == nil && monitor > 0)
		case "PREFIX":
			if value == "" {
				s.prefixModes = ""
				s.prefixSymbols = ""
				break Switch
			}
			if !strings.HasPrefix(value, "(") || len(value)%2 != 0 || len(value) < 2 || value[len(value)/2] != ')' {
				break Switch
			}
			for i := 0; i < len(value); i++ {
				if unicode.MaxASCII < value[i] {
					break Switch
				}
			}
			numPrefixes := len(value)/2 - 1
			s.prefixModes = value[1 : numPrefixes+1]
			s.prefixSymbols = value[numPrefixes+2:]
		case "STATUSMSG":
			s.statusmsg = value
		case "WHOX":
			s.whox = true
		case "SOJU.IM/FILEHOST":
			s.upload = value
		}
	}
}
