package socks5

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// SOCKS5 Protocol Constants (RFC 1928)
const (
	Version5 = 0x05

	// Authentication methods
	AuthNone     = 0x00
	AuthGSSAPI   = 0x01
	AuthPassword = 0x02
	AuthNoAcceptable = 0xFF

	// Commands
	CmdConnect      = 0x01
	CmdBind         = 0x02
	CmdUDPAssociate = 0x03

	// Address types
	AtypIPv4   = 0x01
	AtypDomain = 0x03
	AtypIPv6   = 0x04

	// Reply codes
	ReplySuccess              = 0x00
	ReplyServerFailure        = 0x01
	ReplyConnectionNotAllowed = 0x02
	ReplyNetworkUnreachable   = 0x03
	ReplyHostUnreachable      = 0x04
	ReplyConnectionRefused    = 0x05
	ReplyTTLExpired           = 0x06
	ReplyCommandNotSupported  = 0x07
	ReplyAddressNotSupported  = 0x08
)

// DialWithAuth connects to targetAddr through a SOCKS5 proxy with optional authentication
// proxyAddr: SOCKS5 proxy address (e.g., "proxy.example.com:1080")
// targetAddr: Target destination (e.g., "example.com:80")
// username: Username for authentication (empty string for no auth)
// password: Password for authentication (empty string for no auth)
// timeout: Connection timeout
func DialWithAuth(proxyAddr, targetAddr, username, password string, timeout time.Duration) (net.Conn, error) {
	// Connect to SOCKS5 proxy
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SOCKS5 proxy: %w", err)
	}

	// Set deadline for the entire handshake process
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}

	// Perform SOCKS5 handshake
	if err := performHandshake(conn, targetAddr, username, password); err != nil {
		conn.Close()
		return nil, err
	}

	// Clear deadline after successful handshake
	if timeout > 0 {
		conn.SetDeadline(time.Time{})
	}

	return conn, nil
}

// performHandshake executes the complete SOCKS5 handshake sequence
func performHandshake(conn net.Conn, targetAddr, username, password string) error {
	// Step 1: Negotiate authentication method
	authMethod, err := negotiateAuth(conn, username, password)
	if err != nil {
		return err
	}

	// Step 2: Authenticate if required
	if authMethod == AuthPassword {
		if err := authenticate(conn, username, password); err != nil {
			return err
		}
	}

	// Step 3: Send CONNECT request
	if err := sendConnectRequest(conn, targetAddr); err != nil {
		return err
	}

	return nil
}

// negotiateAuth sends authentication method negotiation (RFC 1928)
// Client sends: [VER][NMETHODS][METHODS...]
// Server replies: [VER][METHOD]
func negotiateAuth(conn net.Conn, username, password string) (byte, error) {
	// Determine which auth methods to offer
	var methods []byte
	methods = append(methods, AuthNone) // Always offer no authentication

	if username != "" || password != "" {
		methods = append(methods, AuthPassword) // Offer username/password auth
	}

	// Build negotiation request
	req := []byte{
		Version5,
		byte(len(methods)),
	}
	req = append(req, methods...)

	// Send negotiation request
	if _, err := conn.Write(req); err != nil {
		return 0, fmt.Errorf("failed to send auth negotiation: %w", err)
	}

	// Read server's choice
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, fmt.Errorf("failed to read auth negotiation response: %w", err)
	}

	if resp[0] != Version5 {
		return 0, fmt.Errorf("invalid SOCKS version: %d", resp[0])
	}

	if resp[1] == AuthNoAcceptable {
		return 0, fmt.Errorf("no acceptable authentication methods")
	}

	return resp[1], nil
}

// authenticate performs username/password authentication (RFC 1929)
// Client sends: [VER][ULEN][UNAME][PLEN][PASSWD]
// Server replies: [VER][STATUS]
func authenticate(conn net.Conn, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return fmt.Errorf("username or password too long (max 255 bytes)")
	}

	// Build authentication request (RFC 1929)
	req := []byte{
		0x01, // VER (username/password auth version)
		byte(len(username)),
	}
	req = append(req, []byte(username)...)
	req = append(req, byte(len(password)))
	req = append(req, []byte(password)...)

	// Send authentication request
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("failed to send authentication: %w", err)
	}

	// Read authentication response
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("failed to read authentication response: %w", err)
	}

	if resp[0] != 0x01 {
		return fmt.Errorf("invalid authentication response version: %d", resp[0])
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("authentication failed: status code %d", resp[1])
	}

	return nil
}

// sendConnectRequest sends SOCKS5 CONNECT command (RFC 1928)
// Client sends: [VER][CMD][RSV][ATYP][DST.ADDR][DST.PORT]
// Server replies: [VER][REP][RSV][ATYP][BND.ADDR][BND.PORT]
func sendConnectRequest(conn net.Conn, targetAddr string) error {
	// Parse target address (host:port)
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("invalid target address: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", portStr)
	}

	// Build CONNECT request
	req := []byte{
		Version5,
		CmdConnect,
		0x00, // Reserved
	}

	// Add address (prefer domain name for better privacy and flexibility)
	if ip := net.ParseIP(host); ip != nil {
		// IP address
		if ipv4 := ip.To4(); ipv4 != nil {
			// IPv4
			req = append(req, AtypIPv4)
			req = append(req, ipv4...)
		} else {
			// IPv6
			req = append(req, AtypIPv6)
			req = append(req, ip...)
		}
	} else {
		// Domain name (ATYP = 0x03)
		if len(host) > 255 {
			return fmt.Errorf("domain name too long: %s", host)
		}
		req = append(req, AtypDomain)
		req = append(req, byte(len(host)))
		req = append(req, []byte(host)...)
	}

	// Add port (big-endian)
	req = append(req, byte(port>>8), byte(port&0xff))

	// Send CONNECT request
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("failed to send CONNECT request: %w", err)
	}

	// Read server reply
	resp := make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("failed to read CONNECT response: %w", err)
	}

	if resp[0] != Version5 {
		return fmt.Errorf("invalid SOCKS version in response: %d", resp[0])
	}

	// Check reply code
	if resp[1] != ReplySuccess {
		return fmt.Errorf("SOCKS5 connection failed: %s", replyCodeString(resp[1]))
	}

	// Read and discard bind address (we don't need it)
	atyp := resp[3]
	var addrLen int
	switch atyp {
	case AtypIPv4:
		addrLen = net.IPv4len
	case AtypIPv6:
		addrLen = net.IPv6len
	case AtypDomain:
		// Read domain length first
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("failed to read domain length: %w", err)
		}
		addrLen = int(lenBuf[0])
	default:
		return fmt.Errorf("unsupported address type: %d", atyp)
	}

	// Read bind address and port
	bindAddrPort := make([]byte, addrLen+2) // address + 2-byte port
	if _, err := io.ReadFull(conn, bindAddrPort); err != nil {
		return fmt.Errorf("failed to read bind address and port: %w", err)
	}

	return nil
}

// replyCodeString returns a human-readable description of SOCKS5 reply codes
func replyCodeString(code byte) string {
	switch code {
	case ReplySuccess:
		return "success"
	case ReplyServerFailure:
		return "general SOCKS server failure"
	case ReplyConnectionNotAllowed:
		return "connection not allowed by ruleset"
	case ReplyNetworkUnreachable:
		return "network unreachable"
	case ReplyHostUnreachable:
		return "host unreachable"
	case ReplyConnectionRefused:
		return "connection refused"
	case ReplyTTLExpired:
		return "TTL expired"
	case ReplyCommandNotSupported:
		return "command not supported"
	case ReplyAddressNotSupported:
		return "address type not supported"
	default:
		return fmt.Sprintf("unknown reply code: 0x%02x", code)
	}
}

// ParseAddress is a helper function to parse host:port strings safely
func ParseAddress(addr string) (host string, port int, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}

	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", portStr)
	}

	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port out of range: %d", port)
	}

	return host, port, nil
}

// Dial connects to targetAddr through a SOCKS5 proxy without authentication
func Dial(proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	return DialWithAuth(proxyAddr, targetAddr, "", "", timeout)
}

// normalizeAddress ensures the address has a port (adds :80 for http, :443 for https)
func normalizeAddress(addr string) string {
	if !strings.Contains(addr, ":") {
		return addr + ":80"
	}
	return addr
}
