package telnet

import "github.com/Jasrags/AnotherMUD/internal/conn"

// *Conn satisfies the GMCP capability interface the session installs the
// inbound handler through (telnet.GmcpHandler is aliased to
// conn.GmcpHandler so SetGmcpHandler matches).
var _ conn.GmcpConn = (*Conn)(nil)
