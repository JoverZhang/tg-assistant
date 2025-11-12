package dialer

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

func CreateProxyDialerFromURL(proxyURL string) (proxy.ContextDialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	switch u.Scheme {
	case "socks5":
		// SOCKS5 proxy
		var auth *proxy.Auth
		if u.User != nil {
			password, _ := u.User.Password()
			auth = &proxy.Auth{
				User:     u.User.Username(),
				Password: password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}

		// Wrap to support DialContext
		return &contextDialer{Dialer: dialer}, nil

	case "http", "https":
		// HTTP proxy
		return &httpProxyDialer{
			proxyURL: u,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s (use socks5 or http)", u.Scheme)
	}
}

// contextDialer wraps a proxy.Dialer to implement proxy.ContextDialer
type contextDialer struct {
	Dialer proxy.Dialer
}

func (d *contextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.Dialer.Dial(network, addr)
}

// httpProxyDialer implements proxy.ContextDialer for HTTP proxies
type httpProxyDialer struct {
	proxyURL *url.URL
}

func (d *httpProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Create a dialer with proxy
	dialer := &net.Dialer{}

	// First connect to proxy
	proxyConn, err := dialer.DialContext(ctx, "tcp", d.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Send HTTP CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("failed to send CONNECT: %w", err)
	}

	// Read response (simplified - just check for 200)
	buf := make([]byte, 1024)
	n, err := proxyConn.Read(buf)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("failed to read proxy response: %w", err)
	}

	// Check for success (HTTP/1.1 200 or HTTP/1.0 200)
	response := string(buf[:n])
	if !strings.Contains(response, "200") {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", response)
	}

	return proxyConn, nil
}
