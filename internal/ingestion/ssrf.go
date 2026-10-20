package ingestion

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var nonPublicPrefixes = parsePrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
	"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	"64:ff9b::/96", "2001::/23", "2001:db8::/32", "2002::/16",
)

func validateSourceURL(source *url.URL) error {
	if source == nil || source.User != nil || source.Hostname() == "" || (source.Scheme != "http" && source.Scheme != "https") {
		return errors.New("source URL is invalid")
	}
	if source.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(source.Port())
	if err != nil || source.Scheme == "http" && port != 80 || source.Scheme == "https" && port != 443 {
		return errors.New("source port is not allowed")
	}
	return nil
}

func resolvePublicAddresses(ctx context.Context, resolver ipResolver, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		if !isPublic(address) {
			return nil, errors.New("source host resolves to a non-public address")
		}
		return []netip.Addr{address}, nil
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("source host could not be resolved")
	}
	for _, address := range addresses {
		if !isPublic(address) {
			return nil, errors.New("source host resolves to a non-public address")
		}
	}
	return addresses, nil
}

func isPublic(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func parsePrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}

func newPinnedTransport(addresses []netip.Addr, source url.URL) *http.Transport {
	return &http.Transport{Proxy: nil, ForceAttemptHTTP2: false, DialContext: pinnedDialer(addresses, source), TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, IdleConnTimeout: time.Second}
}

func pinnedDialer(addresses []netip.Addr, source url.URL) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(host, source.Hostname()) {
			return nil, errors.New("unexpected source dial target")
		}
		if port == "" {
			if source.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}
		var lastErr error
		for _, ip := range addresses {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
}
