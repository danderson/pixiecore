package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

type blobHandler []byte

func (b blobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Log("HTTP", true, "Starting send of %s to %s (%d bytes)", r.URL, r.RemoteAddr, len(b))
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

func ServeHTTP(port int, ldlinux []byte, kernel, initrd, cmdline string) error {
	pxelinuxCfg := fmt.Sprintf(`
DEFAULT linux
LABEL linux
LINUX kernel
APPEND initrd=initrd %s
`, cmdline)

	http.Handle("/ldlinux.c32", blobHandler(ldlinux))
	http.Handle("/pxelinux.cfg/", blobHandler([]byte(pxelinuxCfg)))
	http.Handle("/kernel", fileHandler(kernel))
	http.Handle("/initrd", fileHandler(initrd))

	Log("HTTP", false, "Listening on port %d", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
