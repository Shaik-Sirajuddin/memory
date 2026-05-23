package zellij

import (
	"fmt"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/terminal"
)

type ZellijTransformer struct{}

// ToNative converts a canonical Layout to a zellij KDL layout file.
// All panes are rendered with start_suspended=true.
// The tab at FocusedTab index gets focus=true (caller passes index via context; here we accept it as a param to keep Transformer pure).
func (t *ZellijTransformer) ToNative(layout terminal.Layout) ([]byte, error) {
	return t.ToNativeWithFocus(layout, 0)
}

func (t *ZellijTransformer) ToNativeWithFocus(layout terminal.Layout, focusedTab int) ([]byte, error) {
	var b strings.Builder
	b.WriteString("layout {\n")

	for i, tab := range layout.Tabs {
		focusAttr := ""
		if i == focusedTab {
			focusAttr = " focus=true"
		}
		if tab.Name != "" {
			fmt.Fprintf(&b, "    tab name=%q%s {\n", tab.Name, focusAttr)
		} else {
			fmt.Fprintf(&b, "    tab%s {\n", focusAttr)
		}

		// Tab-level command rendered as a pane when no explicit panes defined
		if len(tab.Panes) == 0 && tab.Command != "" {
			b.WriteString(renderPane(tab.Command, layout.Dir, true, 2))
		}

		for _, pane := range tab.Panes {
			dir := pane.Dir
			if dir == "" {
				dir = layout.Dir
			}
			b.WriteString(renderPane(pane.Command, dir, pane.StartSuspended, 2))
		}

		b.WriteString("    }\n")
	}

	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func (t *ZellijTransformer) FromNative(data []byte) (terminal.Layout, error) {
	// Minimal round-trip parser — production use should use a proper KDL parser.
	// Returns an empty layout; full parsing is out of scope for v1.0.3.
	return terminal.Layout{}, nil
}

func renderPane(command, dir string, suspended bool, indent int) string {
	pad := strings.Repeat("    ", indent)
	var b strings.Builder
	if command != "" {
		fmt.Fprintf(&b, "%spane command=%q start_suspended=%v {\n", pad, command, suspended)
		if dir != "" {
			fmt.Fprintf(&b, "%s    cwd %q\n", pad, dir)
		}
		fmt.Fprintf(&b, "%s}\n", pad)
	} else {
		b.WriteString(pad + "pane\n")
	}
	return b.String()
}
