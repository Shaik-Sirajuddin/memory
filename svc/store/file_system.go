package store

type FileSystem struct {
}

// AppendInstructions implements [Store].
func (f *FileSystem) AppendInstructions(ID InstructionID, params AppendInstructionParams) {
	panic("unimplemented")
}

// DeleteInstructions implements [Store].
func (f *FileSystem) DeleteInstructions(ID InstructionID, params DeleteInstructionParams) {
	panic("unimplemented")
}

// GetInstructions implements [Store].
func (f *FileSystem) GetInstructions(ID InstructionID) {
	panic("unimplemented")
}

// GetInstructionsMeta implements [Store].
func (f *FileSystem) GetInstructionsMeta(ID InstructionID) {
	panic("unimplemented")
}

// GetInstructionsPartial implements [Store].
func (f *FileSystem) GetInstructionsPartial(ID InstructionID, params GetPartialInstructionParams) {
	panic("unimplemented")
}

// SubscribeInstructions implements [Store].
func (f *FileSystem) SubscribeInstructions(ID InstructionID, params SubscriptionParams) {
	panic("unimplemented")
}

// UnSubscribeInstructions implements [Store].
func (f *FileSystem) UnSubscribeInstructions(ID InstructionID) {
	panic("unimplemented")
}

// UpdateInstructions implements [Store].
func (f *FileSystem) UpdateInstructions(ID InstructionID, params UpdateInstructionParams) {
	panic("unimplemented")
}

var _ Store = &FileSystem{}
