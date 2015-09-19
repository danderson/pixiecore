package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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
const limerick = `
	        There once was a protocol called PXE,
	        Whose specification was overly tricksy.
	        A committee refined it,
	        Into a big Turing tarpit,
	        And now you're using it to boot your PC.
`

type blobHandler []byte

func (b blobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Log("HTTP", true, "Starting send of %s to %s (%d bytes)", r.URL, r.RemoteAddr, len(b))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(b)
	Log("HTTP", false, "Sent %s to %s (%d bytes)", r.URL, r.RemoteAddr, len(b))
}

type fileHandler string

func (f fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, err := os.Open(string(f))
	if err != nil {
		Log("HTTP", false, "%s: %s", r.URL, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer h.Close()
	Log("HTTP", true, "Starting send of %s to %s", r.URL, r.RemoteAddr)
	n, err := io.Copy(w, h)
	if err != nil {
		Log("HTTP", false, "%s: %s", r.URL, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	Log("HTTP", false, "Sent %s to %s (%d bytes)", r.URL, r.RemoteAddr, n)
}

type httpServer struct {
	booter Booter
	key    [32]byte // to sign URLs
}

func (s *httpServer) PxelinuxConfig(w http.ResponseWriter, r *http.Request) {
	macStr := filepath.Base(r.URL.Path)

	errStr := fmt.Sprintf("%s requested a pxelinux config from URL %q, which does not include a MAC address", r.RemoteAddr, r.URL)
	if !strings.HasPrefix(macStr, "01-") {
		Log("HTTP", true, errStr)
		http.Error(w, errStr, http.StatusBadRequest)
		return
	}
	mac, err := net.ParseMAC(macStr[3:])
	if err != nil {
		Log("HTTP", true, errStr)
		http.Error(w, errStr, http.StatusBadRequest)
		return
	}

	spec, err := s.booter.BootSpec(mac)
	if err != nil {
		// We have a machine sitting in pxelinux, but the Booter says
		// we shouldn't be netbooting. So, give it a config that tells
		// pxelinux to shut down PXE booting and continue with the
		// next local boot method.
		w.Write([]byte(bootFromDisk))
		Log("HTTP", true, "Telling pxelinux on %s (%s) to boot from disk because of API server verdict: %s", mac, r.RemoteAddr, err)
		return
	}

	spec.Kernel = "f/" + base64.URLEncoding.EncodeToString([]byte(spec.Kernel))
	for i := range spec.Initrd {
		spec.Initrd[i] = "f/" + base64.URLEncoding.EncodeToString([]byte(spec.Initrd[i]))
	}

	cfg := fmt.Sprintf(`
SAY %s
DEFAULT linux
LABEL linux
LINUX %s
APPEND initrd=%s %s
`, strings.Replace(limerick, "\n", "\nSAY ", -1), spec.Kernel, strings.Join(spec.Initrd, ","), spec.Cmdline)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(cfg))
	Log("HTTP", false, "Sent pxelinux config to %s (%s)", mac, r.RemoteAddr)
}

func (s *httpServer) File(w http.ResponseWriter, r *http.Request) {
	encodedID := filepath.Base(r.URL.Path)
	id, err := base64.URLEncoding.DecodeString(encodedID)
	if err != nil {
		Log("http", false, "Bad base64 encoding for URL %q from %s: %s", r.URL, r.RemoteAddr, err)
		http.Error(w, "Malformed file ID", http.StatusBadRequest)
		return
	}
	f, pretty, err := s.booter.File(string(id))
	if err != nil {
		Log("HTTP", false, "Couldn't get byte stream for %q from %s: %s", r.URL, r.RemoteAddr, err)
		http.Error(w, "Couldn't get byte stream", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	written, err := io.Copy(w, f)
	if err != nil {
		Log("HTTP", false, "Error serving %s to %s: %s", pretty, r.RemoteAddr, err)
		return
	}
	Log("HTTP", false, "Sent %s to %s (%d bytes)", pretty, r.RemoteAddr, written)
}

func ServeHTTP(port int, booter Booter, ldlinux []byte) error {
	http.Handle("/ldlinux.c32", blobHandler(ldlinux))

	s := &httpServer{
		booter: booter,
	}
	if _, err := io.ReadFull(rand.Reader, s.key[:]); err != nil {
		return fmt.Errorf("cannot initialize ephemeral signing key: %s", err)
	}

	http.HandleFunc("/pxelinux.cfg/", s.PxelinuxConfig)
	http.HandleFunc("/f/", s.File)

	Log("HTTP", false, "Listening on port %d", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
