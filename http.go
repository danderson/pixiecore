package main

import (
	"fmt"
	"net/http"
)

type blobHandler []byte

func (b blobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Log("HTTP", true, "%s (%d bytes)", r.URL, len(b))
	w.Write(b)
}

func ServeHTTP(port int, ldlinux []byte) error {
	http.Handle("/ldlinux.c32", blobHandler(ldlinux))
	http.HandleFunc("/", Boot)
	Log("HTTP", false, "Listening on port %d", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func Boot(w http.ResponseWriter, r *http.Request) {
	Log("HTTP", true, r.URL.String())
	http.NotFound(w, r)
}
