package tftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"time"
)

const numRetries = 5

type rrq struct {
	Filename string

	FileSize  int
	BlockSize int
}

var Log = func(string, ...interface{}) {}
var Debug = func(string, ...interface{}) {}

func Serve(port int, pxelinux []byte) error {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}

	Log("TFTP", "Listening on port %d", port)
	buf := make([]byte, 512)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			Log("TFTP", "Reading from socket: %s", err)
			continue
		}

		req, err := ParseRRQ(addr, buf[:n])
		if err != nil {
			Debug("TFTP", "ParseRRQ: %s", err)
			conn.WriteTo(TFTPError(err), addr)
			continue
		}

		go transfer(addr, req, pxelinux)
	}
}

func transfer(addr net.Addr, req *rrq, pxelinux []byte) {
	conn, err := net.Dial("udp4", addr.String())
	if err != nil {
		Log("TFTP", "Couldn't set up TFTP socket for %s: %s", addr, err)
		return
	}
	defer conn.Close()

	bsize := 512
	if req.BlockSize > 0 {
		// OACK the blocksize option, ignore all others. Blocksize is
		// implemented purely because it cuts the roundtrip count 3x.
		bsize = req.BlockSize
		pkt := []byte{0, 6}
		pkt = append(pkt, fmt.Sprintf("blksize\x00%d\x00", req.BlockSize)...)
		if err := TFTPData(conn, pkt, 0); err != nil {
			// Some PXE ROMs seem to request a transfer with the tsize
			// option to try and size a buffer, and immediately abort
			// it on OACK. As such, we're going to declare this a
			// debug-level error, because it seems part of a normal
			// boot sequence.
			Debug("TFTP", "Transfer to %s failed: %s", addr, err)
			return
		}
	}

	toTX := pxelinux
	seq := uint16(1)
	buf := make([]byte, bsize+4)
	buf[1] = 3
	for len(toTX) > 0 {
		binary.BigEndian.PutUint16(buf[2:4], seq)
		l := len(toTX)
		if l > bsize {
			l = bsize
		}
		copy(buf[4:], toTX[:l])
		if err = TFTPData(conn, buf[:l+4], seq); err != nil {
			Log("TFTP", "Transfer to %s failed: %s", addr, err)
			return
		}
		seq++
		toTX = toTX[l:]
	}

	Log("TFTP", "Sent pxelinux to %s", addr)
}

func TFTPData(conn net.Conn, b []byte, seq uint16) error {
Tx:
	for try := 0; try < numRetries; try++ {
		conn.Write(b)
		conn.SetReadDeadline(time.Now().Add(time.Second))

		var recv [256]byte
		for {
			n, err := conn.Read(recv[:])
			if err != nil {
				if t, ok := err.(net.Error); ok && t.Timeout() {
					continue Tx
				}
				return err
			}

			if n < 4 {
				continue
			}
			switch binary.BigEndian.Uint16(recv[:2]) {
			case 4:
				if binary.BigEndian.Uint16(recv[2:4]) == seq {
					return nil
				}
			case 5:
				msg, _, _ := nullStr(recv[4:])
				return fmt.Errorf("client aborted transfer (%q)", msg)
			}
		}
	}

	return fmt.Errorf("timed out waiting for ACK #%d", seq)
}

func TFTPError(err error) []byte {
	s := err.Error()
	b := make([]byte, len(s)+5)
	b[1] = 5
	copy(b[4:], s)
	return b
}

func ParseRRQ(addr net.Addr, b []byte) (req *rrq, err error) {
	// Smallest a useful TFTP packet can be is 6 bytes: 2b opcode, 1b
	// filename, 1b null, 1b mode, 1b null.
	if len(b) < 6 {
		return nil, fmt.Errorf("packet from %s too small to be an RRQ", addr)
	}

	if binary.BigEndian.Uint16(b[:2]) != 1 {
		return nil, fmt.Errorf("packet from %s is not an RRQ", addr)
	}

	fname, b, ok := nullStr(b[2:])
	if !ok {
		return nil, fmt.Errorf("request from %s contains no filename", addr)
	}

	mode, b, ok := nullStr(b)
	if !ok {
		return nil, fmt.Errorf("request from %s has no transfer mode", addr)
	}
	if mode != "octet" {
		return nil, fmt.Errorf("%s requested unsupported transfer mode %q", addr, mode)
	}

	req = &rrq{
		Filename: fname,
	}

	for len(b) > 0 {
		opt, valStr := "", ""
		opt, b, ok = nullStr(b)
		if !ok {
			return nil, fmt.Errorf("%s sent unterminated option name", addr)
		}
		valStr, b, ok = nullStr(b)
		if !ok {
			return nil, fmt.Errorf("%s sent unterminated value for option %q", addr, opt)
		}
		val, err := strconv.Atoi(valStr)
		if err != nil {
			return nil, fmt.Errorf("%s sent non-integer %q for option %q", addr, valStr, opt)
		}
		switch opt {
		case "blksize":
			if val < 8 || val > 65464 {
				return nil, fmt.Errorf("%s requested unsupported blocksize %q", addr, val)
			}
			req.BlockSize = val
			// Clamp for use on ethernet. If you're not using
			// ethernet, or are doing crazy encap, you're gonna have a
			// bad time.
			if req.BlockSize > 1450 {
				req.BlockSize = 1450
			}
		}
	}

	return req, nil
}

func nullStr(b []byte) (string, []byte, bool) {
	off := bytes.IndexByte(b, 0)
	if off == -1 {
		return "", nil, false
	}
	return string(b[:off]), b[off+1:], true
}
