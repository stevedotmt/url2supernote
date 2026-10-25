// Package webclient provides the HTTP client used to fetch Articles and their images from the public web.
package webclient

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrNonPublicAddress reports a connection refused because it would reach a loopback, private, link-local,
// shared or otherwise non-public address, or a port other than 80 or 443.
var ErrNonPublicAddress = errors.New("refused to connect to a non-public address")

var allowedPorts = map[uint16]bool{80: true, 443: true}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

// New returns a client that only connects to public addresses on ports 80 and 443. The check runs on the
// resolved address of every connection, so redirects and DNS rebinding cannot reach the local network.
// Proxies from the environment are ignored because the client could not check the address behind them.
func New(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   checkConnection,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext
	return &http.Client{Timeout: timeout, Transport: transport}
}

func checkConnection(_, address string, _ syscall.RawConn) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNonPublicAddress, address)
	}
	if !allowedPorts[addrPort.Port()] || !isPublic(addrPort.Addr()) {
		return fmt.Errorf("%w: %s", ErrNonPublicAddress, addrPort)
	}
	return nil
}

func isPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
