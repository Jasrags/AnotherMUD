package gmcp

// Input.Complete — tab-completion request/response (tab-completion §12,
// Phase 1). Unlike the Char.*/Room.* packages (server→client state
// pushes), this is a client→server *request* with a server→client reply,
// so it lives in its own Input.* namespace.

const (
	// PackageCompleteRequest is the client→server request: the partial
	// input line the player is typing (up to the cursor).
	PackageCompleteRequest = "Input.Complete"

	// PackageCompleteResponse is the server→client reply: the candidate
	// set for the token under completion.
	PackageCompleteResponse = "Input.Complete.List"
)

// CompleteRequest is the client→server payload for PackageCompleteRequest.
type CompleteRequest struct {
	// Line is the partial command line up to the cursor (e.g. "get sw").
	Line string `json:"line"`
}

// CompleteCandidate is one completion option in a CompleteResponse.
type CompleteCandidate struct {
	// Value is the token to insert; it round-trips through ordinary
	// resolution to the thing Display names.
	Value string `json:"value"`
	// Display is the human label (e.g. "a short sword").
	Display string `json:"display"`
	// Kind tags the candidate: verb | item | entity | door | bulk.
	Kind string `json:"kind"`
}

// CompleteResponse is the server→client payload for PackageCompleteResponse.
// Per the §12 policy the client completes to Common (longest common
// prefix) and lists Candidates.
type CompleteResponse struct {
	// Line echoes the request line so the client can match the reply to
	// the input it sent.
	Line string `json:"line"`
	// Target is the slot being completed: "verb" | "argument" | "none".
	Target string `json:"target"`
	// Verb is the resolved verb when Target == "argument" (else empty).
	Verb string `json:"verb,omitempty"`
	// Common is the longest common prefix of the candidate values — what
	// the client completes the token to before showing the list.
	Common string `json:"common"`
	// Truncated is true when the candidate set was capped.
	Truncated bool `json:"truncated"`
	// Candidates is the ordered candidate list (may be empty).
	Candidates []CompleteCandidate `json:"candidates"`
}
