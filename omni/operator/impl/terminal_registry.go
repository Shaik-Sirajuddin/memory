package impl

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/memory/terminal"
	"github.com/Shaik-Sirajuddin/memory/terminal/provider/zellij"
)

type registeredTerminal struct {
	name     string
	terminal terminal.Terminal
}

// terminalRegistry holds the set of known terminal providers keyed by name.
var terminalRegistry = func() []registeredTerminal {
	return []registeredTerminal{
		{name: "zellij", terminal: zellij.New()},
	}
}

// getTerminal returns the terminal.Terminal for the given name, or an error if unknown.
func getTerminal(name string) (terminal.Terminal, error) {
	for _, r := range terminalRegistry() {
		if r.name == name {
			return r.terminal, nil
		}
	}
	return nil, fmt.Errorf("operator: unknown terminal %q; supported: %v", name, supportedTerminalNames())
}

func supportedTerminalNames() []string {
	reg := terminalRegistry()
	names := make([]string, len(reg))
	for i, r := range reg {
		names[i] = r.name
	}
	return names
}
