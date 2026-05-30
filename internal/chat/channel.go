package chat

// Kind classifies a channel's listen/speak gating posture.
//
// Spec: docs/specs/chat-channels-and-tells.md §2.2.
type Kind string

const (
	// KindPublic is the v1 default — anyone tuned in can listen
	// and anyone can speak. No role gating.
	KindPublic Kind = "public"
	// KindGated declares that the channel has speak / listen
	// role-tag gates (§3.3). v1 reads this as a marker but does
	// not yet enforce — see registry_test for the failing-
	// expectation pin. Gates land in M13.6b.
	KindGated Kind = "gated"
)

// Channel describes a single multi-recipient chat topic.
//
// Fields parallel the configuration surface (§10). Today every
// Channel is constructed by the composition root; M13.6b adds a
// YAML loader (file.go in this package).
type Channel struct {
	// ID is the namespaced identifier (e.g. "tapestry-core:ooc").
	// Verb dispatch uses DisplayName, not ID — ID is the registry
	// key and the on-disk addressing convention.
	ID string

	// DisplayName is the player-typed verb and the prefix in
	// rendered output (e.g. "ooc"). Case-insensitive on verb
	// lookup; echoed in canonical form in output.
	DisplayName string

	// Kind is the gating posture (see Kind constants).
	Kind Kind

	// DefaultOn controls whether brand-new characters are auto-
	// tuned. v1 ignores this (everyone is auto-tuned to every
	// channel); preserved on the struct so M13.6b subscription
	// state can read it without a model change.
	DefaultOn bool

	// Persisted toggles whether messages on this channel are
	// written to the on-disk ring buffer (§4.4). v1 keeps every
	// channel's scrollback in memory; this field is honored when
	// the persistence layer lands (M13.6b).
	Persisted bool

	// BufferCap is the per-channel ring-buffer size. 0 means
	// "use the registry default" (see Registry.DefaultBufferCap).
	BufferCap int

	// SpeakGate / ListenGate are the role-tag sets required to
	// speak / listen on a Gated channel (§3.3). Empty for Public.
	// v1 reads but does not enforce.
	SpeakGate  []string
	ListenGate []string
}
