package tui

import (
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/pterm/pterm"
)

var (
	MarkdownRender *glamour.TermRenderer
	BoxStyle       = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			PaddingLeft(1).
			PaddingRight(1).
			MarginLeft(4).
			MarginBottom(1).
			MaxWidth(pterm.GetTerminalWidth() - 4)
)

func markdownBox(contentType, content string) string {
	output, err := MarkdownRender.Render("```" + contentType + "\n" + content + "\n```")
	if err == nil {
		content = output
	}
	return BoxStyle.Render(content)
}

func init() {
	r, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(pterm.GetTerminalWidth()-10),
		glamour.WithStylePath("auto"))
	if err != nil {
		panic(err)
	}
	MarkdownRender = r
}
