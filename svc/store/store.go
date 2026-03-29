package store

type Range struct {
	Start int
	End   int
}

type Rule struct {
	Command string
	Seqno   int
}

type GetPartialInstructionParams struct {
	Range
}

type AppendInstructionParams struct {
	Rules []Rule
}
type UpdateInstructionParams struct {
	Rules []Rule
}

// DeleteInstructionParams removes instructions from global and caller memory
// Caller can refer seqno with respect to caller memory
type DeleteInstructionParams struct {
	SeqNo []int
}

// DiscardInstructionParams removes instructions only from caller specific memory
type DiscardInstructionParams struct {
	SeqNo []int
}

type SubscriptionParams struct {
	// Ignore emitting events for updates perform by caller
	IgnoreSelf bool
}

type InstructionID struct {
	// Bucket is the
	Bucket string
	Path   string
}

// Instructions ,  Rule , Tag
type Store interface {
	GetInstructionsMeta(ID InstructionID)
	GetInstructions(ID InstructionID)
	GetInstructionsPartial(ID InstructionID, params GetPartialInstructionParams)
	AppendInstructions(ID InstructionID, params AppendInstructionParams)
	DeleteInstructions(ID InstructionID, params DeleteInstructionParams)
	UpdateInstructions(ID InstructionID, params UpdateInstructionParams)

	SubscribeInstructions(ID InstructionID, params SubscriptionParams)
	UnSubscribeInstructions(ID InstructionID)
}
