package gmcp

// Char.Commands — the categorized command catalog (ui-rendering-help §10.4,
// networking-protocols §7). A server→client static push shipped once per
// GMCP-active session (like Char.StatusVars), giving a rich/web client the data
// to build clickable command menus without scraping `help` output. The catalog
// is filtered to what the actor can see: the admin group is present only for
// holders of the admin role, matching the bare-help index's role gate.
//
// New-shape package (no Tapestry/Achaea analogue), so keys are chosen here:
// short lowercase keys grouped by category, each command carrying its keyword,
// one-line brief, and a primary syntax line.

// PackageCharCommands is the server→client command-catalog push.
const PackageCharCommands = "Char.Commands"

// CharCommand is one command in a catalog category.
type CharCommand struct {
	// Keyword is the primary verb the client sends (e.g. "kill").
	Keyword string `json:"keyword"`
	// Brief is the one-line description, color markup stripped.
	Brief string `json:"brief,omitempty"`
	// Syntax is the primary usage line (e.g. "put [item] in [container]").
	Syntax string `json:"syntax,omitempty"`
}

// CharCommandCategory is one display group of commands.
type CharCommandCategory struct {
	// Key is the stable category identifier (e.g. "combat") — matches the
	// `help <category>` argument.
	Key string `json:"key"`
	// Title is the human-facing group name (e.g. "Combat").
	Title string `json:"title"`
	// Commands are the group's commands, ordered by keyword. Always an array
	// (never null) so a client can iterate without a nil check.
	Commands []CharCommand `json:"commands"`
}

// CharCommands is the PackageCharCommands payload: the full catalog as an
// ordered list of categories. Categories is always an array.
type CharCommands struct {
	Categories []CharCommandCategory `json:"categories"`
}
