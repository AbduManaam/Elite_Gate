package ipfilter

import (
	"fmt"
	"net"
	"strings"
)

func ExtractIP(remoteAddr string, getHeader func(string) string, trustProxy bool) string {
	if trustProxy && getHeader != nil {
		if xff := getHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(parts[i])
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		if xri := getHeader("X-Real-IP"); xri != "" {
			ip := strings.TrimSpace(xri)
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return ip
}

type IPChecker struct {
	allowedIPs     map[string]bool
	allowedSubnets []*net.IPNet
}

func NewIPChecker(patterns []string) (*IPChecker, error) {
	checker := &IPChecker{
		allowedIPs:     make(map[string]bool),
		allowedSubnets: make([]*net.IPNet, 0),
	}
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if _, ipnet, err := net.ParseCIDR(pat); err == nil {
			checker.allowedSubnets = append(checker.allowedSubnets, ipnet)
		} else if ip := net.ParseIP(pat); ip != nil {
			checker.allowedIPs[ip.String()] = true
		} else {
			return nil, fmt.Errorf("invalid IP/CIDR value: %s", pat)
		}
	}
	return checker, nil
}

func (c *IPChecker) IsAllowed(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if c.allowedIPs[ip.String()] {
		return true
	}
	for _, subnet := range c.allowedSubnets {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func (c *IPChecker) IsBlocked(ipStr string) bool {
	return c.IsAllowed(ipStr)
}
