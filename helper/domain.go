package helper

import (
	"errors"
	"net"
	"strings"
)

var ErrInvalidHostname = errors.New("invalid hostname")

const maxHostnameLength = 253

// NormalizeHostname cleans and validates a customer custom-domain hostname.
//
// Accepted:
//   - api.acmecorp.com
//   - API.AcmeCorp.COM.
//   - api.example.co.uk
//
// Rejected:
//   - https://api.acmecorp.com
//   - api.acmecorp.com/path
//   - api.acmecorp.com:8080
//   - localhost
//   - 192.168.1.10
//   - *.acmecorp.com
//   - acmecorp.com
//
// For the current MVP, only subdomains are accepted.
// Therefore, the hostname must contain at least three labels.
func NormalizeHostname(input string) (string, error) {
	hostname := strings.TrimSpace(input)
	hostname = strings.ToLower(hostname)
	hostname = strings.TrimSuffix(hostname, ".")

	if hostname == "" {
		return "", ErrInvalidHostname
	}

	if len(hostname) > maxHostnameLength {
		return "", ErrInvalidHostname
	}

	// A plain hostname must not contain a URL scheme.
	if strings.Contains(hostname, "://") {
		return "", ErrInvalidHostname
	}

	// Paths and query strings are not allowed.
	if strings.ContainsAny(hostname, "/?#") {
		return "", ErrInvalidHostname
	}

	// Ports and IPv6-style inputs are not allowed.
	if strings.Contains(hostname, ":") {
		return "", ErrInvalidHostname
	}

	// Wildcard domains are not supported in the MVP.
	if strings.Contains(hostname, "*") {
		return "", ErrInvalidHostname
	}

	if hostname == "localhost" {
		return "", ErrInvalidHostname
	}

	// IP addresses are not valid customer hostnames.
	if net.ParseIP(hostname) != nil {
		return "", ErrInvalidHostname
	}

	labels := strings.Split(hostname, ".")

	// MVP rule: accept subdomains such as api.customer.com,
	// but reject apex domains such as customer.com.
	if len(labels) < 3 {
		return "", ErrInvalidHostname
	}

	for _, label := range labels {
		if !validHostnameLabel(label) {
			return "", ErrInvalidHostname
		}
	}

	return hostname, nil
}

func validHostnameLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}

	// A DNS label cannot begin or end with a hyphen.
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}

	for _, char := range label {
		isLowercaseLetter := char >= 'a' && char <= 'z'
		isDigit := char >= '0' && char <= '9'
		isHyphen := char == '-'

		if !isLowercaseLetter && !isDigit && !isHyphen {
			return false
		}
	}

	return true
}
