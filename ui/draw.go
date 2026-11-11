package ui

import (
	"fmt"

	"git.sr.ht/~delthas/senpai/irc"
	"go.rockorager.dev/vaxis"
	"go.rockorager.dev/vaxis/widgets/align"
)

func (ui *UI) Draw(members []irc.Member) {
	ui.clickEvents = ui.clickEvents[:0]

	w, h := ui.vx.window.Size()

	ui.bs.DrawTimeline(ui, ui.channelWidth, 0, ui.config.NickColWidth)
	if ui.channelWidth == 0 {
		ui.bs.DrawHorizontalBufferList(ui.vx, 0, h-1, w-ui.memberWidth, &ui.channelOffset)
	} else {
		ui.bs.DrawVerticalBufferList(ui.vx, 0, 0, ui.channelWidth, h, &ui.channelOffset)
	}
	if ui.memberWidth != 0 {
		ui.drawVerticalMemberList(ui.vx, w-ui.memberWidth, 0, ui.memberWidth, h, ui.bs.cur(), members, &ui.memberOffset)
	}
	if ui.channelWidth == 0 {
		ui.drawStatusBar(ui.channelWidth, h-3, w-ui.memberWidth)
	} else {
		ui.drawStatusBar(ui.channelWidth, h-2, w-ui.channelWidth-ui.memberWidth)
	}

	prompt := ui.prompt
	if ui.bs.HasOverlay() && ui.e.Empty() {
		prompt = Styled(">", vaxis.Style{
			Foreground: ui.config.Colors.Prompt,
		})
	}
	if ui.channelWidth == 0 {
		for x := 0; x < 9+ui.config.NickColWidth; x++ {
			setCell(ui.vx, x, h-2, ' ', vaxis.Style{})
		}
		printIdent(ui.vx, 7, h-2, ui.config.NickColWidth, prompt)
	} else {
		for x := ui.channelWidth; x < 9+ui.channelWidth+ui.config.NickColWidth; x++ {
			setCell(ui.vx, x, h-1, ' ', vaxis.Style{})
		}
		printIdent(ui.vx, ui.channelWidth+7, h-1, ui.config.NickColWidth, prompt)
	}

	var hint string
	if ui.bs.HasOverlay() {
		hint = ui.overlayHint
	}
	if ui.channelWidth == 0 {
		ui.e.Draw(ui.vx, 9+ui.config.NickColWidth, h-2, hint)
	} else {
		ui.e.Draw(ui.vx, 9+ui.channelWidth+ui.config.NickColWidth, h-1, hint)
	}

	if ui.image != nil {
		iw, ih := ui.image.CellSize()
		ui.image.Draw(align.Center(ui.vx.window, iw, ih))
	}

	ui.vx.Render()
}

func (ui *UI) drawHorizontalLine(vx *Vaxis, x0, y, width int) {
	for x := x0; x < x0+width; x++ {
		setCell(vx, x, y, '─', vaxis.Style{
			Foreground: ui.config.Colors.Gray,
		})
	}
}

func (ui *UI) drawVerticalLine(vx *Vaxis, x, y0, height int) {
	for y := y0; y < y0+height; y++ {
		setCell(vx, x, y, '│', vaxis.Style{})
	}
}

func (ui *UI) drawStatusBar(x0, y, width int) {
	clearArea(ui.vx, x0, y, width, 1)

	if ui.status == "" {
		return
	}

	var s StyledStringBuilder
	s.SetStyle(vaxis.Style{
		Foreground: ui.config.Colors.Gray,
	})
	s.WriteString("--")

	x := x0 + 5 + ui.config.NickColWidth
	printString(ui.vx, &x, y, s.StyledString())
	x += 2

	s.Reset()
	s.SetStyle(vaxis.Style{
		Foreground: ui.config.Colors.Gray,
	})
	s.WriteString(ui.status)

	printString(ui.vx, &x, y, s.StyledString())
}

func (ui *UI) drawVerticalMemberList(vx *Vaxis, x0, y0, width, height int, b *buffer, members []irc.Member, offset *int) {
	ui.drawVerticalLine(vx, x0, y0, height)
	x0++
	width--
	clearArea(vx, x0, y0, width, height)

	if _, channel := ui.bs.Current(); channel == "" && ui.config.Mouse {
		x := x0 + 1
		printString(vx, &x, y0, Styled("Help", vaxis.Style{
			Foreground: ui.config.Colors.Status,
		}))
		ui.drawHorizontalLine(vx, x0, y0+1, width)
		y0 += 2

		lines := []string{
			"→Add network",
			"→Join channel",
			"→Message user",
		}
		for i, line := range lines {
			var st vaxis.Style
			if i*2 == ui.memberClicked {
				st.Attribute |= vaxis.AttrReverse
			}
			x := x0
			printString(vx, &x, y0, Styled(line, st))
			ui.drawHorizontalLine(vx, x0, y0+1, width)
			y0 += 2
		}
		return
	}

	if len(members) > 0 {
		var memberString string
		if len(members) > 1 {
			memberString = fmt.Sprintf("%d members", len(members))
		} else {
			memberString = fmt.Sprintf("%d member", len(members))
		}
		memberString = truncate(vx, memberString, width-1, "\u2026")
		xMembers := x0 + 1
		printString(vx, &xMembers, y0, Styled(memberString, vaxis.Style{
			Foreground: ui.config.Colors.Status,
		}))
	}
	y0++
	height--
	ui.drawHorizontalLine(vx, x0, y0, width)
	y0++
	height--

	if ui.config.Mouse {
		var actions []string
		if b.muted {
			actions = append(actions, "→ Unmute")
		} else {
			actions = append(actions, "→ Mute")
		}
		if b.pinned {
			actions = append(actions, "→ Unpin")
		} else {
			actions = append(actions, "→ Pin")
		}
		actions = append(actions, "→ Leave")
		for i, action := range actions {
			x := x0
			ui.drawHorizontalLine(vx, x0, y0+height-(len(actions)-i)*2, width)
			printString(vx, &x, y0+height-(len(actions)-i)*2+1, Styled(action, vaxis.Style{}))
		}
		height -= 2 * len(actions)
	}

	padding := 1
	for _, m := range members {
		if m.Disconnected {
			padding = runeWidth(vx, 0x274C)
			break
		}
	}

	if y0+len(members)-*offset < height {
		*offset = y0 + len(members) - height
		if *offset < 0 {
			*offset = 0
		}
	}

	for i, m := range members[*offset:] {
		if i >= height {
			break
		}
		var attr vaxis.AttributeMask
		if i+*offset == ui.memberClicked {
			attr |= vaxis.AttrReverse
		}
		x := x0
		y := y0 + i
		if m.Disconnected {
			disconnectedSt := vaxis.Style{
				Foreground: ColorRed,
				Attribute:  attr,
			}
			printString(vx, &x, y, Styled("\u274C", disconnectedSt))
		} else if m.PowerLevel != "" {
			x += padding - 1
			powerLevelText := m.PowerLevel[:1]
			powerLevelSt := vaxis.Style{
				Foreground: ColorGreen,
				Attribute:  attr,
			}
			printString(vx, &x, y, Styled(powerLevelText, powerLevelSt))
		} else {
			x += padding
		}

		var name StyledString
		nameText := truncate(vx, m.Name.Name, width-1, "\u2026")
		if m.Away {
			name = Styled(nameText, vaxis.Style{
				Foreground: ui.config.Colors.Gray,
				Attribute:  attr,
			})
		} else {
			color := ui.IdentColor(ui.config.Colors.Nicks, m.Name.Name, m.Self)
			name = Styled(nameText, vaxis.Style{
				Foreground: color,
				Attribute:  attr,
			})
		}

		printString(vx, &x, y, name)
	}
}
