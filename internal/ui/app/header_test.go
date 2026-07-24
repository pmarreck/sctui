package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestRenderHeaderUsesTerminalWidth(t *testing.T) {
	for _, width := range []int{50, 120} {
		t.Run("width", func(t *testing.T) {
			application := NewAppWithDependencies(nil, nil, nil)
			application.width = width

			headerLines := strings.Split(application.renderHeader(), "\n")
			assert.Equal(t, width, lipgloss.Width(headerLines[0]))

			separatorFound := false
			for _, line := range headerLines {
				if strings.Contains(line, "─") {
					separatorFound = true
					assert.Equal(t, width, strings.Count(line, "─"))
				}
			}
			assert.True(t, separatorFound)
		})
	}
}
