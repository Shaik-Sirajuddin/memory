package hooks

type Manager interface {
	// init calls registers default hooks required by manager
	Init()

	// Callback
	Callback(Execution any) error
}

