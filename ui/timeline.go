package ui

import (
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/events"
	"go.rockorager.dev/vaxis"
)

func (bs *BufferList) forEachLine(b *buffer, f func(line *Line, y int) bool) bool {
	rulerDrawn := b.unreadSkip != optionalFalse || b.unreadRuler.IsZero() || b.title == ""
	y := 0
	for i := len(b.lines) - 1; 0 <= i; i-- {
		line := &b.lines[i]
		if !rulerDrawn && !line.At.After(b.unreadRuler) {
			rulerDrawn = true
			y++
		}
		if f(line, y) {
			return true
		}
		y += len(line.NewLines(bs.ui.vx, bs.textWidth)) + 1
	}
	return false
}

func (bs *BufferList) shouldShowDate(b *buffer, i int, yi int, y0 int) bool {
	if i == 0 || yi <= y0 {
		return true
	}
	yb, mb, dd := b.lines[i-1].At.Local().Date()
	ya, ma, da := b.lines[i].At.Local().Date()
	return yb != ya || mb != ma || dd != da
}

func (bs *BufferList) DrawTimeline(ui *UI, x0, y0, nickColWidth int) {
	vx := ui.vx
	clearArea(vx, x0, y0, bs.tlInnerWidth+nickColWidth+9, bs.tlHeight+2)

	b := bs.cur()
	if !b.openedOnce {
		b.openedOnce = true
		for i := 0; i < len(b.lines); i++ {
			b.lines[i].Body = b.lines[i].Body.ParseURLs()
		}
	}

	for b.topicOffset > 0 {
		sr := []rune(b.topic.string)
		ri := 0
		for i := 0; i < b.topicOffset; i++ {
			s, _ := firstCluster(bs.ui.vx, sr[ri:])
			ri += len([]rune(s))
		}
		w := stringWidth(bs.ui.vx, string(sr[ri:]))
		if w <= bs.tlInnerWidth+nickColWidth+9-16 {
			b.topicOffset -= 12
			if b.topicOffset < 0 {
				b.topicOffset = 0
			}
		} else {
			break
		}
	}

	xTopic := x0
	{
		// TODO: factorize this (same code for drawing timeline)

		var st vaxis.Style
		nextStyles := b.topic.styles

		sr := []rune(b.topic.string)
		ri := 0
		for i := 0; i < b.topicOffset; i++ {
			s, _ := firstCluster(bs.ui.vx, sr[ri:])
			ri += len([]rune(s))
		}
		i := len(string(sr[:ri]))
		sr = sr[ri:]
		for len(sr) > 0 {
			if 0 < len(nextStyles) && nextStyles[0].Start == i {
				st = nextStyles[0].Style
				nextStyles = nextStyles[1:]

				if (bs.ui.mouseLinks || st.Hyperlink == "") && st.HyperlinkParams != "" && st.UnderlineStyle == 0 {
					st.UnderlineStyle = vaxis.UnderlineDotted
				}
			}
			dx, di := printCluster(vx, xTopic, y0, -1, sr, st)
			xTopic += dx
			i += len(string(sr[:di]))
			sr = sr[di:]

			if st.Hyperlink != "" {
				ui.clickEvents = append(ui.clickEvents, clickEvent{
					xb: xTopic - dx,
					xe: xTopic,
					y:  y0,
					event: &events.EventClickLink{
						EventClick: events.EventClick{
							NetID:  b.netID,
							Buffer: b.title,
						},
						Link:  st.Hyperlink,
						Mouse: ui.mouseLinks,
					},
				})
			} else if _, channel, ok := strings.Cut(st.HyperlinkParams, "="); ok {
				ui.clickEvents = append(ui.clickEvents, clickEvent{
					xb: xTopic - dx,
					xe: xTopic,
					y:  y0,
					event: &events.EventClickChannel{
						EventClick: events.EventClick{
							NetID:  b.netID,
							Buffer: b.title,
						},
						Channel: channel,
					},
				})
			}
		}
	}
	y0++
	bs.ui.drawHorizontalLine(vx, x0, y0, bs.tlInnerWidth+nickColWidth+9)
	y0++

	if bs.textWidth < bs.tlInnerWidth {
		x0 += (bs.tlInnerWidth - bs.textWidth) / 2
	}

	yi := b.scrollAmt + y0 + bs.tlHeight
	rulerDrawn := b.unreadSkip != optionalFalse || b.unreadRuler.IsZero() || b.title == ""
	for i := len(b.lines) - 1; 0 <= i; i-- {
		if yi < y0 {
			break
		}

		x1 := x0 + 9 + nickColWidth

		line := &b.lines[i]
		nls := line.NewLines(bs.ui.vx, bs.textWidth)

		if !rulerDrawn {
			isRead := !line.At.After(b.unreadRuler)
			if isRead && yi > y0 {
				yi--
				st := vaxis.Style{
					Foreground: bs.ui.config.Colors.Gray,
				}
				printIdent(vx, x0+7, yi, nickColWidth, Styled("--", st))
				bs.ui.drawHorizontalLine(vx, x0, yi, 9+nickColWidth+bs.tlInnerWidth)
				rulerDrawn = true
			}
		}

		yi -= len(nls) + 1
		if y0+bs.tlHeight <= yi {
			continue
		}

		showDate := bs.shouldShowDate(b, i, yi, y0)
		if showDate {
			st := vaxis.Style{
				Attribute: vaxis.AttrBold,
			}
			// as a special case, always draw the first visible message date, even if it is a continuation line
			yd := yi
			if yd < y0 {
				yd = y0
			}
			printDate(vx, x0, yd, st, line.At.Local())
		} else {
			showTime := b.lines[i-1].At.Truncate(time.Minute) != line.At.Truncate(time.Minute) && yi >= y0
			if !showTime {
				// also try to show the time if we previously drew the date
				yp := yi - (len(b.lines[i-1].NewLines(bs.ui.vx, bs.textWidth)) + 1)
				showTime = i == 0 || bs.shouldShowDate(b, i-1, yp, y0)
			}
			if showTime {
				st := vaxis.Style{
					Foreground: bs.ui.config.Colors.Gray,
				}
				printTime(vx, x0, yi, st, line.At.Local())
			}
		}

		if yi >= y0 {
			head := line.Head
			if line.Highlight && len(line.Head.styles) > 0 {
				var sb StyledStringBuilder
				sb.WriteString(head.string)
				for _, st := range line.Head.styles {
					s := st.Style
					s.Attribute |= vaxis.AttrReverse
					sb.AddStyle(st.Start, s)
				}
				head = sb.StyledString()
			}
			xb, xe := printIdent(vx, x0+7, yi, nickColWidth, head)

			lastHead := line.Head.string
			if len(line.Head.styles) > 0 {
				lastHead = lastHead[line.Head.styles[len(line.Head.styles)-1].Start:]
			}

			if !strings.HasSuffix(lastHead, "--") && !strings.HasSuffix(lastHead, "!!") && lastHead != "*" {
				ui.clickEvents = append(ui.clickEvents, clickEvent{
					xb: xb,
					xe: xe,
					y:  yi,
					event: &events.EventClickNick{
						EventClick: events.EventClick{
							NetID:  b.netID,
							Buffer: b.title,
						},
						Nick: lastHead,
					},
				})
			}
		}

		x := x1
		y := yi
		var style vaxis.Style
		nextStyles := line.Body.styles

		lbi := 0
		l := []rune(line.Body.string)
		for len(l) > 0 {
			if 0 < len(nextStyles) && nextStyles[0].Start == lbi {
				style = nextStyles[0].Style
				nextStyles = nextStyles[1:]

				if (bs.ui.mouseLinks || style.Hyperlink == "") && style.HyperlinkParams != "" && style.UnderlineStyle == 0 {
					style.UnderlineStyle = vaxis.UnderlineDotted
				}
			}
			if 0 < len(nls) && lbi == nls[0] {
				x = x1
				y++
				nls = nls[1:]
				if y0+bs.tlHeight <= y {
					break
				}
			}

			if l[0] == '\n' {
				lbi++
				l = l[1:]
				continue
			}
			if y != yi && x == x1 && IsSplitRune(l[0]) {
				lbi += len(string(l[0]))
				l = l[1:]
				continue
			}

			xb := x
			if y >= y0 {
				dx, di := printCluster(vx, x, y, -1, l, style)
				x += dx
				lbi += len(string(l[:di]))
				l = l[di:]
			} else {
				c, cw := firstCluster(vx, l)
				x += cw
				lbi += len(c)
				l = l[len([]rune(c)):]
			}

			if style.Hyperlink != "" {
				ui.clickEvents = append(ui.clickEvents, clickEvent{
					xb: xb,
					xe: x,
					y:  y,
					event: &events.EventClickLink{
						EventClick: events.EventClick{
							NetID:  b.netID,
							Buffer: b.title,
						},
						Link:  style.Hyperlink,
						Mouse: ui.mouseLinks,
					},
				})
			} else if _, channel, ok := strings.Cut(style.HyperlinkParams, "="); ok {
				ui.clickEvents = append(ui.clickEvents, clickEvent{
					xb: xb,
					xe: x,
					y:  y,
					event: &events.EventClickChannel{
						EventClick: events.EventClick{
							NetID:  b.netID,
							Buffer: b.title,
						},
						Channel: channel,
					},
				})
			}
		}
	}

	b.isAtTop = y0 <= yi
}
