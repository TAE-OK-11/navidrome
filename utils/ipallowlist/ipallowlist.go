package ipallowlist

import (
	"fmt"
	"net"
	"slices"
	"strings"
)

// Contains reports whether ip is contained in comma-separated CIDR entries.
// An empty list or empty ip returns false. The special entry "@" matches unix
// peer addresses when unixSocket is true.
func Contains(ip, commaSeparatedList string, unixSocket bool) bool {
	if commaSeparatedList == "" || ip == "" {
		return false
	}

	cidrs := strings.Split(commaSeparatedList, ",")

	if ip == "@" && unixSocket {
		return slices.Contains(cidrs, "@")
	}

	if net.ParseIP(ip) == nil {
		ip, _, _ = net.SplitHostPort(ip)
	}
	if ip == "" {
		return false
	}

	testedIP, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ip))
	if err != nil {
		return false
	}

	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil && ipnet.Contains(testedIP) {
			return true
		}
	}
	return false
}
