package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
)

// pxelinux configuration that tells the PXE/UNDI stack to boot from
// local disk.
const bootFromDisk = `
DEFAULT local
LABEL local
LOCALBOOT 0
`

// A silly limerick displayed while pxelinux loads big OS
// images. Possibly the most important piece of this program.
const limerick = `
	        There once was a protocol called PXE,
	        Whose specification was overly tricksy.
	        A committee refined it,
	        Into a big Turing tarpit,
	        And now you're using it to boot your PC.
`

type httpServer struct {
	booter  Booter
	ldlinux []byte
	key     [32]byte // to sign URLs
	port    int
}

func (s *httpServer) Ldlinux(w http.ResponseWriter, r *http.Request) {
	macCookie, err := r.Cookie("_Syslinux_BOOTIF")
	mac, err := getMac(macCookie.String())
	if err != nil {
		Debug("HTTP", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	Debug("HTTP", "Checking whether to boot %v", mac)
	err = s.booter.ShouldBoot(mac)
	if err != nil {
		Debug("HTTP", "Telling pxelinux on %s (%s) to boot from disk because of API server verdict: %s", mac, r.RemoteAddr, err)
		w.Write([]byte(bootFromDisk))
		return
	}

	Debug("HTTP", "Starting send of ldlinux.c32 to %s (%d bytes)", r.RemoteAddr, len(s.ldlinux))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(s.ldlinux)
	Log("HTTP", "Sent ldlinux.c32 to %s (%d bytes)", r.RemoteAddr, len(s.ldlinux))
}

func (s *httpServer) PxelinuxConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	macStr := filepath.Base(r.URL.Path)
	errStr := fmt.Sprintf("%s requested a pxelinux config from URL %q, which does not include a MAC address", r.RemoteAddr, r.URL)
	mac, err := getMac(macStr)
	if err != nil {
		Debug("HTTP", errStr)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, _, err := net.SplitHostPort(r.Host); err != nil {
		r.Host = fmt.Sprintf("%s:%d", r.Host, s.port)
	}

	spec, err := s.booter.BootSpec(mac, fmt.Sprintf("http://%s/f/", r.Host))
	if err != nil {
		// We have a machine sitting in pxelinux, but the Booter says
		// we shouldn't be netbooting. So, give it a config that tells
		// pxelinux to shut down PXE booting and continue with the
		// next local boot method.
		Debug("HTTP", "Telling pxelinux on %s (%s) to boot from disk because of API server verdict: %s", mac, r.RemoteAddr, err)
		w.Write([]byte(bootFromDisk))
		return
	}

	msg := limerick
	if spec.Message != "" {
		msg = spec.Message
	}
	var cfg bytes.Buffer
	fmt.Fprintf(&cfg, `SAY %s
DEFAULT linux
LABEL linux
KERNEL %s
`, strings.Replace(msg, "\n", "\nSAY ", -1), spec.Kernel)
	args := spec.Cmdline
	if len(spec.Initrd) > 0 {
		args = fmt.Sprintf("initrd=%s %s", strings.Join(spec.Initrd, ","), args)
	}
	if args != "" {
		fmt.Fprintf(&cfg, "APPEND %s", args)
	}
	if _, err := io.Copy(w, &cfg); err != nil {
		Log("HTTP", "Error writing pxelinux configuration: %s", err)
	}
	Log("HTTP", "Sent pxelinux config to %s (%s)", mac, r.RemoteAddr)
}

func getMac(macStr string) (mac net.HardwareAddr, err error) {
	if !strings.HasPrefix(macStr, "01-") {
		return nil, fmt.Errorf("Missing MAC address in request")
	}
	mac, err = net.ParseMAC(macStr[3:])
	if err != nil {
		return nil, fmt.Errorf("Malformed MAC address in request")
	}

	return mac, err
}

func (s *httpServer) File(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)

	var (
		f      io.ReadCloser
		pretty string
		err    error
	)
	if r.Method == "POST" {
		f, pretty, err = s.booter.Write(id, r.Body)
	} else {
		f, pretty, err = s.booter.Read(id)
	}
	if err != nil {
		Log("HTTP", "Couldn't get byte stream for %q from %s: %s", r.URL, r.RemoteAddr, err)
		http.Error(w, "Couldn't get byte stream", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	written, err := io.Copy(w, f)
	if err != nil {
		Log("HTTP", "Error serving %s to %s: %s", pretty, r.RemoteAddr, err)
		return
	}
	Log("HTTP", "Sent %s to %s (%d bytes)", pretty, r.RemoteAddr, written)
}

func serveHTTP(addr string, port int, booter Booter, ldlinux []byte) error {
	s := &httpServer{
		booter:  booter,
		ldlinux: ldlinux,
		port:    port,
	}
	if _, err := io.ReadFull(rand.Reader, s.key[:]); err != nil {
		return fmt.Errorf("cannot initialize ephemeral signing key: %s", err)
	}

	http.HandleFunc("/ldlinux.c32", s.Ldlinux)
	http.HandleFunc("/pxelinux.cfg/", s.PxelinuxConfig)
	http.HandleFunc("/f/", s.File)

	httpAddr := fmt.Sprintf("%s:%d", addr, port)
	Log("HTTP", "Listening on %s", httpAddr)
	return http.ListenAndServe(httpAddr, nil)
}
