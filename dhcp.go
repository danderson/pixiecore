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

type DHCPPacket struct {
	TID  []byte
	MAC  net.HardwareAddr
	GUID []byte

	ServerIP net.IP
	//HTTPServer string
}

func ServeProxyDHCP(port int) error {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	l := ipv4.NewPacketConn(conn)
	if err = l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return err
	}

	Log("ProxyDHCP", false, "Listening on port %d", port)
	buf := make([]byte, 1024)
	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			Log("ProxyDHCP", false, "Error reading from socket: %s", err)
			continue
		}

		udpAddr := addr.(*net.UDPAddr)
		udpAddr.IP = net.IPv4bcast

		req, err := ParseDHCP(buf[:n])
		if err != nil {
			Log("ProxyDHCP", true, "ParseDHCP: %s", err)
			continue
		}

		// TODO: figure out the correct IP
		req.ServerIP = net.ParseIP("192.168.16.10").To4()
		//req.HTTPServer = fmt.Sprintf("http://%s:%d/", req.ServerIP, httpPort)

		if _, err := l.WriteTo(OfferDHCP(req), &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, udpAddr); err != nil {
			Log("ProxyDHCP", false, "Responding to %s: %s", req.MAC, err)
			continue
		}
		Log("ProxyDHCP", false, "Offering to boot %s", req.MAC)
	}
}

var pxeMenuOffer struct {
	base []byte

	tidOff      int
	macOff      int
	serverIPOff int
	guidOff     int
	bootOff     int
}

func init() {
	r := make([]byte, 236)
	r[0] = 2     // boot reply
	r[1] = 1     // PHY = ethernet
	r[2] = 6     // Hardware address length
	r[10] = 0x80 // Please speak broadcast
	pxeMenuOffer.tidOff = 4
	pxeMenuOffer.macOff = 28

	// DHCP magic
	r = append(r, dhcpMagic...)
	// DHCPOFFER
	r = append(r, 53, 1, 2)
	// Server ID (IP filled in by OfferDHCP)
	r = append(r, 54, 4, 0, 0, 0, 0)
	pxeMenuOffer.serverIPOff = len(r) - 4
	// Vendor class
	r = append(r, 60, 9)
	r = append(r, "PXEClient"...)
	// Client UUID (GUID filled in by OfferDHCP)
	r = append(r, 97, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	pxeMenuOffer.guidOff = len(r) - 16

	p := []byte{}
	// PXE discovery control - disable broadcast and multicast discovery
	p = append(p, 6, 1, 3)
	// PXE boot servers (IP filled in by OfferDHCP)
	p = append(p, 8, 7, 0x80, 0x00, 1, 0, 0, 0, 0)
	bootOff := len(p) - 4
	// PXE boot menu
	p = append(p, 9, 12, 0x80, 0x00, 9)
	p = append(p, "Pixiecore"...)
	// PXE menu prompt/soapbox text
	p = append(p, 10, 10, 0)
	p = append(p, "Pixiecore"...)

	// PXE vendor options wrapper
	r = append(r, 43, byte(len(p)+1))
	r = append(r, p...)
	pxeMenuOffer.bootOff = len(r) - len(p) + bootOff
	r = append(r, 255)

	// Done!
	pxeMenuOffer.base = append(r, 255)
}

func OfferDHCP(p *DHCPPacket) []byte {
	r := append([]byte(nil), pxeMenuOffer.base...)
	copy(r[pxeMenuOffer.tidOff:], p.TID)
	copy(r[pxeMenuOffer.macOff:], p.MAC)
	copy(r[pxeMenuOffer.serverIPOff:], p.ServerIP)
	copy(r[pxeMenuOffer.guidOff:], p.GUID)
	copy(r[pxeMenuOffer.bootOff:], p.ServerIP)
	return r
}

func ParseDHCP(b []byte) (req *DHCPPacket, err error) {
	if len(b) < 240 {
		return nil, errors.New("packet too short")
	}

	ret := &DHCPPacket{
		TID: b[4:8],
		MAC: net.HardwareAddr(b[28:34]),
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
		case 97:
			if len(val) != 17 || val[0] != 0 {
				return nil, fmt.Errorf("packet from %s has malformed option 97", ret.MAC)
			}
			ret.GUID = val[1:]
		}
		typ, val, opts = dhcpOption(opts)
	}

	if ret.GUID == nil {
		return nil, fmt.Errorf("%s is not a PXE client", ret.MAC)
	}

	// Valid PXE request!
	return ret, nil
}

func dhcpOption(b []byte) (typ byte, val []byte, next []byte) {
	if len(b) < 2 || b[0] == 255 {
		return 255, nil, nil
	}
	typ, l := b[0], int(b[1])
	if len(b) < l+2 {
		return 255, nil, nil
	}
	return typ, b[2 : 2+l], b[2+l:]
}
