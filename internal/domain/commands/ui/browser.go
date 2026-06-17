package ui

import (
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
)

// browserURL builds the page URL to open for a bound listener. An unspecified
// bind host (0.0.0.0 / ::) is rewritten to 127.0.0.1 so the browser connects to
// loopback. A non-empty query is passed via ?sql= so the editor opens
// pre-filled.
func browserURL(addr net.Addr, query string) string {
	host := "127.0.0.1"
	port := ""
	if tcp, ok := addr.(*net.TCPAddr); ok {
		port = strconv.Itoa(tcp.Port)
		if tcp.IP != nil && !tcp.IP.IsUnspecified() {
			host = tcp.IP.String()
		}
	}

	u := "http://" + net.JoinHostPort(host, port) + "/"
	if query != "" {
		u += "?sql=" + url.QueryEscape(query)
	}
	return u
}

// openBrowser opens rawURL in the user's default browser using the
// platform-appropriate launcher. It returns the launcher's start error, if any.
func openBrowser(rawURL string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{rawURL}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		name, args = "xdg-open", []string{rawURL}
	}
	return exec.Command(name, args...).Start()
}
