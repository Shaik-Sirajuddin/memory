package sandbox

type FilesPolicy struct {
	AccessDirs  []string // regex supported
	BlockedDirs []string // regex supported
}

type ExtendedPolicy struct {
	// dir defaults to "" , requires user to specify absolute paths
	dir string // ""
	FilesPolicy
}

type Workspace struct {
	Dir string
	*FilesPolicy
}

type Sandbox struct {
	WorkSpace      *Workspace
	ExtendedPolicy *ExtendedPolicy
}
