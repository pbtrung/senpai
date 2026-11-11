package irc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.rockorager.dev/vaxis"
)

const (
	maxMultilineBytes   = 64 * 1024
	maxMultilineLines   = 1024
	maxMultilineBatches = 16
)

type multilineBatch struct {
	opener       Message
	command      string
	text         strings.Builder
	lines, bytes int
	err          error
}

func (s *Session) multilineLimits() (bytes, lines int) {
	bytes, lines = maxMultilineBytes, maxMultilineLines
	for _, field := range strings.Split(s.availableCaps["draft/multiline"], ",") {
		key, value, ok := strings.Cut(field, "=")
		n, err := strconv.Atoi(value)
		if !ok || err != nil || n <= 0 {
			continue
		}
		switch key {
		case "max-bytes":
			if n < bytes {
				bytes = n
			}
		case "max-lines":
			if n < lines {
				lines = n
			}
		}
	}
	return
}

// handleMultiline reconstructs a logical message before history/search routing,
// preserving the opener's parent batch, timestamp and other message tags.
func (s *Session) handleMultiline(msg Message) (bool, *Message, error) {
	if msg.Command == "BATCH" && len(msg.Params) > 0 {
		ref := msg.Params[0]
		if strings.HasPrefix(ref, "+") && len(msg.Params) > 1 && msg.Params[1] == "draft/multiline" {
			if len(ref) == 1 || len(msg.Params) != 3 || len(s.multilineBatches) >= maxMultilineBatches {
				return true, nil, fmt.Errorf("invalid multiline batch")
			}
			id := ref[1:]
			if _, exists := s.multilineBatches[id]; exists {
				return true, nil, fmt.Errorf("duplicate multiline batch %q", id)
			}
			s.multilineBatches[id] = &multilineBatch{opener: msg}
			return true, nil, nil
		}
		if strings.HasPrefix(ref, "-") {
			if b, ok := s.multilineBatches[ref[1:]]; ok {
				delete(s.multilineBatches, ref[1:])
				if b.err != nil {
					return true, nil, b.err
				}
				if b.lines == 0 {
					return true, nil, fmt.Errorf("empty multiline batch")
				}
				assembled := Message{Prefix: b.opener.Prefix, Tags: b.opener.Tags,
					Command: b.command, Params: []string{b.opener.Params[2], b.text.String()}}
				return true, &assembled, nil
			}
		}
	}
	b, ok := s.multilineBatches[msg.Tags["batch"]]
	if !ok {
		return false, nil, nil
	}
	if b.err != nil {
		return true, nil, nil
	}
	_, concat := msg.Tags["draft/multiline-concat"]
	if (msg.Command != "PRIVMSG" && msg.Command != "NOTICE") || len(msg.Params) != 2 ||
		(b.command != "" && msg.Command != b.command) || msg.Params[0] != b.opener.Params[2] ||
		msg.Prefix.String() != b.opener.Prefix.String() || (concat && msg.Params[1] == "") {
		b.err = fmt.Errorf("invalid multiline fragment")
		return true, nil, nil
	}
	b.lines++
	b.bytes += len(msg.Params[1])
	// Bound memory independently of the remote's advertised sending limits.
	if b.bytes > maxMultilineBytes || b.lines > maxMultilineLines {
		b.err = fmt.Errorf("multiline batch exceeds receive limits")
		b.text.Reset()
		return true, nil, nil
	}
	if b.lines > 1 && !concat {
		b.text.WriteByte('\n')
	}
	b.text.WriteString(msg.Params[1])
	b.command = msg.Command
	return true, nil, nil
}

func (s *Session) sendText(command, target, text string, lineLen int) {
	// Carriage returns must never reach the line-oriented IRC transport.
	text = strings.ReplaceAll(text, "\r", "")
	maxBytes, maxLines := s.multilineLimits()
	if lineLen < 1 {
		lineLen = 1
	}
	if s.HasCapability("draft/multiline") && maxBytes < lineLen {
		lineLen = maxBytes
	}
	type fragment struct {
		text   string
		concat bool
	}
	var fragments []fragment
	for _, line := range strings.Split(text, "\n") {
		chunks := splitChunks(line, lineLen)
		for i, chunk := range chunks {
			fragments = append(fragments, fragment{chunk, i > 0})
		}
	}
	if !s.HasCapability("draft/multiline") || len(fragments) <= 1 {
		for _, f := range fragments {
			if f.text != "" {
				s.send(NewMessage(command, target, f.text))
			}
		}
		return
	}
	for len(fragments) > 0 {
		n, bytes := 0, 0
		for n < len(fragments) && n < maxLines && bytes+len(fragments[n].text) <= maxBytes {
			bytes += len(fragments[n].text)
			n++
		}
		// A grapheme can exceed the server's byte limit. Fall back to a single
		// message instead of looping forever or sending an invalid batch.
		if n == 0 {
			s.send(NewMessage(command, target, fragments[0].text))
			fragments = fragments[1:]
			continue
		}
		if bytes == 0 {
			fragments = fragments[n:]
			continue
		}
		s.multilineID++
		id := "senpai" + strconv.FormatUint(s.multilineID, 10)
		s.send(NewMessage("BATCH", "+"+id, "draft/multiline", target))
		for i, f := range fragments[:n] {
			msg := NewMessage(command, target, f.text).WithTag("batch", id)
			if i > 0 && f.concat {
				msg = msg.WithTag("draft/multiline-concat", "")
			}
			s.send(msg)
		}
		s.send(NewMessage("BATCH", "-"+id))
		fragments = fragments[n:]
	}
}

func splitChunks(s string, chunkLen int) (chunks []string) {
	if chunkLen <= 0 || len(s) <= chunkLen {
		return []string{s}
	}

	b := 0
	n := 0
	it := vaxis.NewCharacterIterator(s)
	for {
		c, ok := it.Next()
		if !ok {
			break
		}
		cw := len(c.Grapheme)
		if cw > chunkLen {
			if n > 0 {
				chunks = append(chunks, s[b:b+n])
				b += n
				n = 0
			}
			// A single grapheme can contain arbitrarily many combining marks.
			// Fall back to code point boundaries so it cannot exceed IRC LINELEN.
			for cw > 0 {
				size := min(chunkLen, cw)
				if size < cw {
					for size > 0 && !utf8.RuneStart(s[b+size]) {
						size--
					}
				}
				if size == 0 {
					_, size = utf8.DecodeRuneInString(s[b:])
				}
				chunks = append(chunks, s[b:b+size])
				b += size
				cw -= size
			}
			continue
		}
		if n+cw > chunkLen {
			chunks = append(chunks, s[b:b+n])
			b += n
			n = cw
			continue
		}
		n += cw
	}
	if b < len(s) {
		chunks = append(chunks, s[b:])
	}
	return
}
