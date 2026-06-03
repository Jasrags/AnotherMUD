package telnet

import "github.com/Jasrags/AnotherMUD/internal/conn"

// *Conn satisfies the GMCP capability interface the session installs the
// inbound handler through (telnet.GmcpHandler is aliased to
// conn.GmcpHandler so SetGmcpHandler matches).
var _ conn.GmcpConn = (*Conn)(nil)

// *Conn also implements the char-mode capability the session installs the
// completion provider + toggle through.
var _ conn.CharModeConn = (*Conn)(nil)
