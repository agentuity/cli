package gravity

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

func getPrivateIPv4() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get private IPv4: %w", err)
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no private IPv4 address found")
}

func sendCORSHeaders(headers http.Header) {
	headers.Set("access-control-allow-origin", "*")
	headers.Set("access-control-expose-headers", "Content-Type")
	headers.Set("access-control-allow-headers", "Content-Type, Authorization")
	headers.Set("access-control-allow-methods", "GET, POST, OPTIONS")
}

func isConnectionErrorRetryable(err error) bool {
	if strings.Contains(err.Error(), "connection refused") {
		return true
	}
	if strings.Contains(err.Error(), "connection reset by peer") {
		return true
	}
	if strings.Contains(err.Error(), "No connection could be made because the target machine actively refused it") { // windows
		return true
	}
	return false
}
