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

type PXEPacket struct {
	TID  []byte
	MAC  net.HardwareAddr
	GUID []byte

	ServerIP   net.IP
	HTTPServer string
}

func ServePXE(port, httpPort int) error {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	l := ipv4.NewPacketConn(conn)
	if err = l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return err
	}

	Log("PXE", false, "Listening on port %d", port)
	buf := make([]byte, 1024)
	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			Log("PXE", false, "Error reading from socket: %s", err)
			continue
		}

		udpAddr := addr.(*net.UDPAddr)
		udpAddr.IP = net.IPv4bcast

		req, err := ParsePXE(buf[:n])
		if err != nil {
			Log("PXE", true, "ParsePXE: %s", err)
			continue
		}

		// TODO: figure out the correct IP
		req.ServerIP = net.ParseIP("192.168.16.10").To4()
		req.HTTPServer = fmt.Sprintf("http://%s:%d/", req.ServerIP, httpPort)

		if _, err := l.WriteTo(OfferPXE(req), &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, udpAddr); err != nil {
			Log("PXE", false, "Responding to %s: %s", req.MAC, err)
			continue
		}
		Log("PXE", false, "Offering to boot %s", req.MAC)
	}
}

func OfferPXE(p *PXEPacket) []byte {
	// Base DHCP response
	r := make([]byte, 240)
	r[0] = 2 // boot reply
	r[1] = 1 // PHY = ethernet
	r[2] = 6 // hardware address length
	copy(r[4:8], p.TID)
	r[10] = 0x80 // Please speak broadcast
	copy(r[28:34], p.MAC)
	copy(r[44:107], p.ServerIP.String())
	// The actual filename doesn't matter, the TFTP server serves the
	// pxelinux binary for any request.
	copy(r[108:108+127], "pxelinux")
	copy(r[236:240], dhcpMagic)

	// DHCPOFFER
	r = append(r, 53, 1, 2)
	// Server ID
	r = append(r, 54, 4)
	r = append(r, p.ServerIP...)
	// Vendor class
	r = append(r, 60, 9)
	r = append(r, "PXEClient"...)
	// Client UUID
	r = append(r, 97, 17, 0)
	r = append(r, p.GUID...)
	// PXE discovery control == "just TFTP the damn file"
	r = append(r, 43, 4, 6, 1, 1<<3, 255)
	// PXELinux path prefix (steer to HTTP)
	r = append(r, 210, byte(len(p.HTTPServer)))
	r = append(r, p.HTTPServer...)
	// Done
	r = append(r, 255)

	return r
}

func ParsePXE(b []byte) (req *PXEPacket, err error) {
	if len(b) < 240 {
		return nil, errors.New("packet too short")
	}

	ret := &PXEPacket{
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

	typ, val, opts := option(b[240:])
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
		typ, val, opts = option(opts)
	}

	if ret.GUID == nil {
		return nil, fmt.Errorf("%s is not a PXE client", ret.MAC)
	}

	// Valid PXE request!
	return ret, nil
}

func option(b []byte) (typ byte, val []byte, next []byte) {
	if len(b) < 2 || b[0] == 255 {
		return 255, nil, nil
	}
	typ, l := b[0], int(b[1])
	if len(b) < l+2 {
		return 255, nil, nil
	}
	return typ, b[2 : 2+l], b[2+l:]
}
