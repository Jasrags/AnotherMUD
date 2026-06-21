package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantKey string
		wantVal string
		wantOK  bool
		wantErr bool
	}{
		{name: "simple", in: "FOO=bar", wantKey: "FOO", wantVal: "bar", wantOK: true},
		{name: "spaces around equals", in: "FOO = bar", wantKey: "FOO", wantVal: "bar", wantOK: true},
		{name: "export prefix", in: "export FOO=bar", wantKey: "FOO", wantVal: "bar", wantOK: true},
		{name: "blank", in: "   ", wantOK: false},
		{name: "comment", in: "# a comment", wantOK: false},
		{name: "double quoted", in: `FOO="hello world"`, wantKey: "FOO", wantVal: "hello world", wantOK: true},
		{name: "double quoted escapes", in: `FOO="a\nb"`, wantKey: "FOO", wantVal: "a\nb", wantOK: true},
		{name: "single quoted literal", in: `FOO='a\nb'`, wantKey: "FOO", wantVal: `a\nb`, wantOK: true},
		{name: "inline comment unquoted", in: "FOO=bar # trailing", wantKey: "FOO", wantVal: "bar", wantOK: true},
		{name: "hash inside quotes kept", in: `FOO="a#b"`, wantKey: "FOO", wantVal: "a#b", wantOK: true},
		{name: "empty value", in: "FOO=", wantKey: "FOO", wantVal: "", wantOK: true},
		{name: "csv value", in: "PACKS=wot,tapestry-core", wantKey: "PACKS", wantVal: "wot,tapestry-core", wantOK: true},
		{name: "missing equals", in: "FOOBAR", wantErr: true},
		{name: "empty key", in: "=bar", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val, ok, err := parseLine(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseLine(%q) expected error, got nil", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLine(%q) unexpected error: %v", tt.in, err)
			}
			if ok != tt.wantOK {
				t.Fatalf("parseLine(%q) ok=%v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && (key != tt.wantKey || val != tt.wantVal) {
				t.Fatalf("parseLine(%q) = (%q,%q), want (%q,%q)", tt.in, key, val, tt.wantKey, tt.wantVal)
			}
		})
	}
}

func TestLoad_SetsUnsetKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "ANOTHERMUD_TEST_ADDR=:5000\n# comment\nANOTHERMUD_TEST_PACKS=wot\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANOTHERMUD_TEST_ADDR", "")  // ensure clean slate; Setenv auto-restores
	os.Unsetenv("ANOTHERMUD_TEST_ADDR")   // truly unset so Load fills it
	t.Setenv("ANOTHERMUD_TEST_PACKS", "") // register for cleanup
	os.Unsetenv("ANOTHERMUD_TEST_PACKS")

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("ANOTHERMUD_TEST_ADDR"); got != ":5000" {
		t.Errorf("ADDR = %q, want :5000", got)
	}
	if got := os.Getenv("ANOTHERMUD_TEST_PACKS"); got != "wot" {
		t.Errorf("PACKS = %q, want wot", got)
	}
}

func TestLoad_RealEnvWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ANOTHERMUD_TEST_WIN=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANOTHERMUD_TEST_WIN", "from-env")

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("ANOTHERMUD_TEST_WIN"); got != "from-env" {
		t.Errorf("real env should win: got %q, want from-env", got)
	}
}

func TestLoad_MissingFileIsNoError(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
}

func TestLoad_MalformedLineErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("VALID=ok\nNOEQUALS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err == nil {
		t.Error("expected error for malformed line, got nil")
	}
}
