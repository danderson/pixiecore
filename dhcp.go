package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

var dhcpMagic = []byte{99, 130, 83, 99}

type dhcpPacket struct {
	TID      []byte
	MAC      net.HardwareAddr
	GUID     []byte
	giaddr   net.IP
	ServerIP net.IP
}

func serveProxyDHCP(addr string, booter Booter) error {
	conn, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	l := ipv4.NewPacketConn(conn)
	if err = l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return err
	}

	Log("ProxyDHCP", "Listening on %s", addr)
	buf := make([]byte, 1024)
	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			Log("ProxyDHCP", "Error reading from socket: %s", err)
			continue
		}

		udpAddr := addr.(*net.UDPAddr)
		udpAddr.Port = 68

		req, err := parseDHCP(buf[:n])
		if err != nil {
			Debug("ProxyDHCP", "parseDHCP: %s", err)
			continue
		}

		if err = booter.ShouldBoot(req.MAC); err != nil {
			Debug("ProxyDHCP", "Not offering to boot %s: %s", req.MAC, err)
			continue
		}

		req.ServerIP, err = interfaceIP(msg.IfIndex)
		if err != nil {
			Log("ProxyDHCP", "Couldn't find an IP address to use to reply to %s: %s", req.MAC, err)
			continue
		}

		Log("ProxyDHCP", "Offering to boot %s (via %s)", req.MAC, req.ServerIP)
		if _, err := l.WriteTo(offerDHCP(req), &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, udpAddr); err != nil {
			Log("ProxyDHCP", "Responding to %s: %s", req.MAC, err)
			continue
		}
	}
}

func offerDHCP(p *dhcpPacket) []byte {
	var b bytes.Buffer

	// Fixed length BOOTP response
	var bootp [236]byte
	bootp[0] = 2     // BOOTP reply
	bootp[1] = 1     // PHY = ethernet
	bootp[2] = 6     // Hardware address length
	bootp[10] = 0x80 // Please speak broadcast
	copy(bootp[4:], p.TID)
	copy(bootp[24:], p.giaddr)
	copy(bootp[28:], p.MAC)

	b.Write(bootp[:])

	// DHCP magic
	b.Write(dhcpMagic)
	// Type = DHCPOFFER
	b.Write([]byte{53, 1, 2})
	// Server ID
	b.Write([]byte{54, 4})
	b.Write(p.ServerIP)
	// Vendor class
	b.Write([]byte{60, 9})
	b.WriteString("PXEClient")
	// Client UUID
	if p.GUID != nil {
		b.Write([]byte{97, 17, 0})
		b.Write(p.GUID)
	}

	// PXE vendor options
	var pxe bytes.Buffer
	// Discovery Control - disable broadcast and multicast boot server discovery
	pxe.Write([]byte{6, 1, 3})
	// PXE boot server
	pxe.Write([]byte{8, 7, 0x80, 0x00, 1})
	pxe.Write(p.ServerIP)
	// PXE boot menu - one entry, pointing to the above PXE boot server
	pxe.Write([]byte{9, 12, 0x80, 0x00, 9})
	pxe.WriteString("Pixiecore")
	// PXE menu prompt+timeout
	pxe.Write([]byte{10, 10, 0})
	pxe.WriteString("Pixiecore")
	// End vendor options
	pxe.WriteByte(255)
	b.Write([]byte{43, byte(pxe.Len())})
	pxe.WriteTo(&b)

	// End DHCP options
	b.WriteByte(255)

	return b.Bytes()
}

func parseDHCP(b []byte) (req *dhcpPacket, err error) {
	if len(b) < 240 {
		return nil, errors.New("packet too short")
	}

	ret := &dhcpPacket{
		TID: b[4:8],
		MAC: net.HardwareAddr(b[28:34]),
		giaddr: net.IP(b[24:30]),
	}

	// BOOTP operation type
	if b[0] != 1 {
		return nil, fmt.Errorf("packet from %s is not a BOOTP request", ret.MAC)
	}
	if b[1] != 1 && b[2] != 6 {
		return nil, fmt.Errorf("packet from %s is not for an Ethernet PHY", ret.MAC)
	}
	if !bytes.Equal(b[236:240], dhcpMagic) {
		return nil, fmt.Errorf("packet from %s is not a DHCP request", ret.MAC)
	}

	hasArch := false
	typ, val, opts := dhcpOption(b[240:])
	for typ != 255 {
		switch typ {
		case 53:
			if len(val) != 1 {
				return nil, fmt.Errorf("packet from %s has malformed option 53", ret.MAC)
			}
			if val[0] != 1 {
				return nil, fmt.Errorf("packet from %s is not a DHCPDISCOVER", ret.MAC)
			}
		case 93:
			if len(val) != 2 {
				return nil, fmt.Errorf("packet from %s has malformed option 93", ret.MAC)
			}
			if binary.BigEndian.Uint16(val) != 0 {
				return nil, fmt.Errorf("%s is not an x86 PXE client", ret.MAC)
			}
			hasArch = true
		case 97:
			if len(val) != 17 || val[0] != 0 {
				return nil, fmt.Errorf("packet from %s has malformed option 97", ret.MAC)
			}
			ret.GUID = val[1:]
		}
		typ, val, opts = dhcpOption(opts)
	}

	// Use DHCP option 93 as a proxy for "is this a PXE request?"
	if !hasArch {
		return nil, fmt.Errorf("packet from %s is not a PXE request (missing option 93, 'client architecture')", ret.MAC)
	}

	// Valid PXE request!
	return ret, nil
}

func dhcpOption(b []byte) (typ byte, val []byte, next []byte) {
	if len(b) < 2 || b[0] == 255 {
		return 255, nil, nil
	}
	typ, l := b[0], int(b[1])
	if len(b) < l + 2 {
		return 255, nil, nil
	}
	return typ, b[2 : 2 + l], b[2 + l:]
}

func interfaceIP(ifIdx int) (net.IP, error) {
	iface, err := net.InterfaceByIndex(ifIdx)
	if err != nil {
		return nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	// Try to find an IPv4 address to use, in the following order:
	// global unicast (includes rfc1918), link-local unicast,
	// loopback.
	fs := [](func(net.IP) bool){
		net.IP.IsGlobalUnicast,
		net.IP.IsLinkLocalUnicast,
		net.IP.IsLoopback,
	}
	for _, f := range fs {
		for _, a := range addrs {
			ipaddr, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipaddr.IP.To4()
			if ip == nil {
				continue
			}
			if f(ip) {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("interface %s has no usable unicast addresses", iface.Name)
}
