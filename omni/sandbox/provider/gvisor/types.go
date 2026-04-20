package gvisor

type runtimeEntry struct {
	ID     string
	PID    string
	Status string
}

type ociMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options,omitempty"`
}

type ociNamespace struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

type ociLinux struct {
	Namespaces  []ociNamespace `json:"namespaces,omitempty"`
	MaskedPaths []string       `json:"maskedPaths,omitempty"`
}

type ociSpec struct {
	OCIVersion string     `json:"ociVersion"`
	Mounts     []ociMount `json:"mounts,omitempty"`
	Process    struct {
		Terminal bool     `json:"terminal"`
		Args     []string `json:"args"`
		Cwd      string   `json:"cwd"`
	} `json:"process"`
	Root struct {
		Path     string `json:"path"`
		Readonly bool   `json:"readonly"`
	} `json:"root"`
	Linux *ociLinux `json:"linux,omitempty"`
}
