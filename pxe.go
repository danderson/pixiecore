package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

type PXEPacket struct {
	DHCPPacket
	ClientIP        net.IP
	PXEVendorOption []byte // The bytes for DHCP option 43, for the response.

	HTTPServer string
}

func ServePXE(pxePort, httpPort int) error {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", pxePort))
	if err != nil {
		return err
	}
	l := ipv4.NewPacketConn(conn)
	if err = l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return err
	}

	Log("PXE", false, "Listening on port %d", pxePort)
	buf := make([]byte, 1024)
	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			Log("PXE", false, "Error reading from socket: %s", err)
			continue
		}

		req, err := ParsePXE(buf[:n])
		if err != nil {
			Log("PXE", true, "ParsePXE: %s", err)
			continue
		}

		// TODO: figure out the correct IP
		req.ServerIP, err = interfaceIP(msg.IfIndex)
		req.HTTPServer = fmt.Sprintf("http://%s:%d/", req.ServerIP, httpPort)

		Log("PXE", false, "Chainloading %s to pxelinux (via %s)", req.MAC, req.ServerIP)

		if _, err := l.WriteTo(ReplyPXE(req), &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, addr); err != nil {
			Log("PXE", false, "Responding to %s: %s", req.MAC, err)
			continue
		}
	}
}

func ReplyPXE(p *PXEPacket) []byte {
	r := make([]byte, 236)
	r[0] = 2 // boot reply
	r[1] = 1 // PHY = ethernet
	r[2] = 6 // MAC address length
	copy(r[4:], p.TID)
	r[10] = 0x80 // speak broadcast
	copy(r[16:], p.ClientIP)
	copy(r[20:], p.ServerIP)
	copy(r[28:], p.MAC)
	// Boot file name. Our TFTP server unconditionally serves up
	// pxelinux no matter the name, so we just put something that
	// looks nice in packet dumps.
	copy(r[108:], "boot")

	// DHCP magic
	r = append(r, dhcpMagic...)
	// DHCPACK
	r = append(r, 53, 1, 5)
	// Server ID
	r = append(r, 54, 4)
	r = append(r, p.ServerIP...)
	// Vendor class
	r = append(r, 60, 9)
	r = append(r, "PXEClient"...)
	// Client UUID
	r = append(r, 97, 17, 0)
	r = append(r, p.GUID...)
	// Mirror the menu selection back at the client
	r = append(r, p.PXEVendorOption...)
	// Pxelinux path prefix, which makes pxelinux use HTTP for
	// everything.
	r = append(r, 210, byte(len(p.HTTPServer)))
	r = append(r, p.HTTPServer...)

	// Done.
	r = append(r, 255)

	return r
}

func ParsePXE(b []byte) (req *PXEPacket, err error) {
	if len(b) < 240 {
		return nil, errors.New("packet too short")
	}

	ret := &PXEPacket{
		DHCPPacket: DHCPPacket{
			TID: b[4:8],
			MAC: net.HardwareAddr(b[28:34]),
		},
		ClientIP: net.IP(b[12:16]),
	}

	// We do lighter packet verification here, because the PXE port
	// should not have random unrelated traffic on it, and if there
	// is, the clients deserve everything they get.
	if !bytes.Equal(b[236:240], dhcpMagic) {
		return nil, fmt.Errorf("packet from %s is not a DHCP request", ret.MAC)
	}

	typ, val, opts := dhcpOption(b[240:])
	for typ != 255 {
		switch typ {
		case 43:
			pxeTyp, pxeVal, val := dhcpOption(val)
		pxeOptParse:
			for pxeTyp != 255 {
				if pxeTyp == 71 {
					ret.PXEVendorOption = []byte{43, byte(len(pxeVal) + 3), 71, byte(len(pxeVal))}
					ret.PXEVendorOption = append(ret.PXEVendorOption, pxeVal...)
					ret.PXEVendorOption = append(ret.PXEVendorOption, 255)
					break pxeOptParse
				}
				pxeTyp, pxeVal, val = dhcpOption(val)
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
	if ret.PXEVendorOption == nil {
		return nil, fmt.Errorf("%s hasn't selected a menu option", ret.MAC)
	}

	// Valid PXE request!
	return ret, nil
}
