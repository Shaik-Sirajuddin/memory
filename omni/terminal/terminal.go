package terminal

type Tab struct {
	Name    string
	Command string
}

// Provision terminal with no duplicate Names
type ProvisionParams struct {
	Name string
	Tabs []Tab
}

type Terminal interface {
	Provider() string
	Provision(ProvisionParams) error
	ListSessions()
}
