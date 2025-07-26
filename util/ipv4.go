package util

import (
	"encoding/binary"
	"net"
	"strings"
)

type Ipv4 uint32

func NewIpv4(ip_str string) Ipv4 {
	ip := net.ParseIP(strings.TrimSpace(ip_str))
	if len(ip) == 16 {
		return Ipv4(binary.BigEndian.Uint32(ip[12:16]))
	}
	return Ipv4(binary.BigEndian.Uint32(ip))
}

func (i Ipv4) String() string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, uint32(i))
	return ip.String()
}
