// Package dotenv provides a tiny, dependency-free loader for `.env` files.
//
// It exists so every ANOTHERMUD_* knob can be set from a single local file
// instead of being exported into the shell. The loader is deliberately
// minimal: it populates os.Environ only for keys that are NOT already present,
// so a real environment variable always wins over the file. That ordering
// keeps the existing os.LookupEnv / envOr config readers in cmd/anothermud
// working unchanged — the file is just an additional source layered *under*
// the process environment.
//
// Supported syntax (one entry per line):
//
//	KEY=value
//	export KEY=value          # optional leading `export`
//	KEY="quoted value"        # double quotes; \n \t \\ \" escapes honored
//	KEY='quoted value'        # single quotes; value taken literally
//	# a comment line          # blank lines and #-lines are ignored
//	KEY=value   # trailing    # inline comments after an UNQUOTED value
//
// A missing file is not an error — `.env` is optional.
package dotenv

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load reads key=value pairs from the file at path and sets each into the
// process environment via os.Setenv, but only for keys that are not already
// set (the real environment takes precedence). A non-existent file returns
// nil — callers treat `.env` as optional. Parse errors (a line with no `=`)
// are reported with the offending line number so a typo is loud, not silent.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening env file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		key, val, ok, err := parseLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if !ok {
			continue // blank or comment line
		}
		if _, present := os.LookupEnv(key); present {
			continue // real environment wins
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("%s:%d: setting %s: %w", path, lineNo, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading env file %s: %w", path, err)
	}
	return nil
}

// parseLine parses a single `.env` line. It returns ok=false for blank lines
// and comments, and an error for a non-blank line that has no `=` separator.
func parseLine(raw string) (key, val string, ok bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	line = strings.TrimPrefix(line, "export ")

	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false, fmt.Errorf("missing '=' in entry %q", raw)
	}
	key = strings.TrimSpace(line[:eq])
	if key == "" {
		return "", "", false, fmt.Errorf("empty key in entry %q", raw)
	}
	val = unquote(strings.TrimSpace(line[eq+1:]))
	return key, val, true, nil
}

// unquote strips surrounding quotes from a value. A double-quoted value has
// common escapes expanded; a single-quoted value is taken literally. An
// unquoted value has any trailing inline comment (whitespace + `#`) removed.
func unquote(v string) string {
	if len(v) >= 2 {
		if v[0] == '"' && v[len(v)-1] == '"' {
			inner := v[1 : len(v)-1]
			r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\\`, `\`)
			return r.Replace(inner)
		}
		if v[0] == '\'' && v[len(v)-1] == '\'' {
			return v[1 : len(v)-1]
		}
	}
	// Unquoted: strip an inline comment that begins after whitespace.
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}
