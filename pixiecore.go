package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"

	"golang.org/x/net/ipv4"
)

func main() {
	log.Fatalln(ServePXE())
}

var dhcpMagic = []byte{99, 130, 83, 99}

type MalformedPacket string

func (m MalformedPacket) Error() string {
	return "malformed DHCP packet: " + string(m)
}

type UninterestingPacket error

func (u UninterestingPacket) Error() string {
	return "uninteresting DHCP packet: " + string(m)
}

type PXEPacket struct {
	TID  []byte
	MAC  []byte
	GUID []byte

	ServerIP net.IP
	Filename string
}

func ServePXE() error {
	conn, err := net.ListenPacket("udp4", ":67")
	if err != nil {
		return err
	}
	l := ipv4.NewPacketConn(conn)
	if err = l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return err
	}

	buf := make([]byte, 1024)
	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			fmt.Println(addr.String(), err)
			continue
		}

		udpAddr := addr.(*net.UDPAddr)
		udpAddr.IP = net.IPv4bcast

		req, err := ParsePXE(buf[:n])
		if err != nil {
			fmt.Println(addr.String(), err)
			continue
		}

		req.ServerIP = net.ParseIP("192.168.16.10")
		req.Filename = "undionly.kpxe.0"

		b, err := OfferPXE(req)
		if err != nil {
			fmt.Println(addr.String(), err)
			continue
		}

		if _, err := l.WriteTo(b, &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, udpAddr); err != nil {
			fmt.Println(addr.String(), err)
			continue
		}
	}
}

func OfferPXE(p *PXEPacket) ([]byte, error) {
	p.ServerIP = p.ServerIP.To4()
	if p.ServerIP == nil {
		return nil, errors.New("need an IPv4 address for the server")
	}
	if len(p.Filename) > 127 {
		return nil, errors.New("filename is too long for PXE response")
	}

	// Base DHCP response
	r := make([]byte, 240)
	r[0] = 2 // boot reply
	r[1] = 1 // PHY = ethernet
	r[2] = 6 // hardware address length
	copy(r[4:8], p.TID)
	r[10] = 0x80 // Please speak broadcast
	copy(r[28:34], p.MAC)
	copy(r[44:107], p.ServerIP.String())
	copy(r[108:108+127], p.Filename)
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
	// Done
	r = append(r, 255)

	return r, nil
}

func ParsePXE(b []byte) (req *PXEPacket, err error) {
	if len(b) < 240 {
		return nil, MalformedPacket("packet too short")
	}
	// BOOTP operation type
	if b[0] != 1 {
		return nil, UninterestingPacket("not a BOOTP request packet")
	}
	if b[1] != 1 && b[2] != 6 {
		return nil, MalformedPacket("not Ethernet")
	}
	if !bytes.Equal(b[236:240], dhcpMagic) {
		return nil, UninterestingPacket("not a DHCP request")
	}

	ret := &PXEPacket{
		TID: b[4:8],
		MAC: b[28:34],
	}

	typ, val, opts := option(b[240:])
	for typ != 255 {
		switch typ {
		case 53:
			if len(val) != 1 {
				return nil, MalformedPacket("wrong value size for option 53")
			}
			if val[0] != 1 {
				return nil, UninterestingPacket("not a DHCPOFFER")
			}
		case 93:
			if len(val) != 2 {
				return nil, MalformedPacket("wrong value size for option 93")
			}
			if binary.BigEndian.Uint16(val) != 0 {
				return nil, UninterestingPacket("not an x86 PXE client")
			}
		case 97:
			if len(val) != 17 {
				return nil, MalformedPacket("wrong value size for option 97")
			}
			if val[0] != 0 {
				return nil, MalformedPacket("client identifier not a GUID")
			}
			ret.GUID = val[1:]
		}
		typ, val, opts = option(opts)
	}

	if ret.GUID == nil {
		return nil, UninterestingPacket("not a PXE request")
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
