package senpai

import (
	"sort"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

func commandDoHelp(app *App, args []string) (err error) {
	t := time.Now()
	netID, buffer := app.win.CurrentBuffer()

	addLineCommand := func(sb *ui.StyledStringBuilder, name string, cmd *command) {
		sb.Reset()
		sb.Grow(len(name) + 1 + len(cmd.Usage))
		sb.SetStyle(vaxis.Style{
			Attribute: vaxis.AttrBold,
		})
		sb.WriteString(name)
		sb.SetStyle(vaxis.Style{})
		sb.WriteByte(' ')
		sb.WriteString(cmd.Usage)
		app.win.AddLine(netID, buffer, ui.Line{
			At:   t,
			Body: sb.StyledString(),
		})
		app.win.AddLine(netID, buffer, ui.Line{
			At:   t,
			Body: ui.PlainSprintf("  %s", cmd.Desc),
		})
	}

	addLineCommands := func(names []string) {
		sort.Strings(names)
		var sb ui.StyledStringBuilder
		for _, name := range names {
			addLineCommand(&sb, name, commands[name])
		}
	}

	if len(args) == 0 {
		app.win.AddLine(netID, buffer, ui.Line{
			At:   t,
			Head: ui.PlainString("--"),
			Body: ui.PlainString("Available commands:"),
		})

		cmdNames := make([]string, 0, len(commands))
		for cmdName := range commands {
			cmdNames = append(cmdNames, cmdName)
		}
		addLineCommands(cmdNames)
	} else {
		search := strings.ToUpper(args[0])
		app.win.AddLine(netID, buffer, ui.Line{
			At:   t,
			Head: ui.PlainString("--"),
			Body: ui.PlainSprintf("Commands that match \"%s\":", search),
		})

		cmdNames := make([]string, 0, len(commands))
		for cmdName := range commands {
			if !strings.Contains(cmdName, search) {
				continue
			}
			cmdNames = append(cmdNames, cmdName)
		}
		if len(cmdNames) == 0 {
			app.win.AddLine(netID, buffer, ui.Line{
				At:   t,
				Body: ui.PlainSprintf("  no command matches %q", args[0]),
			})
		} else {
			addLineCommands(cmdNames)
		}
	}
	return nil
}
