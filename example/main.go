// This is an example API server that just statically serves a kernel,
// initrd and commandline. This is effectively the same as Pixiecore
// in static mode, only it's talking to an API server instead.
//
// This is not production-quality code. The focus is on being short
// and sweet, not robust and correct.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	port          = flag.Int("port", 4242, "Port to listen on")
	kernelFile    = flag.String("kernel", "", "Path to the linux kernel file to boot")
	initrdFile    = flag.String("initrd", "", "Path to the initrd to pass to the kernel")
	kernelCmdline = flag.String("cmdline", "", "Additional arguments for the kernel commandline")
	cloudInit     = flag.String("cloud-init", "", "Cloud init file to load")
)

func main() {
	flag.Parse()
	http.HandleFunc("/v1/boot/", API)
	http.HandleFunc("/kernel", func(w http.ResponseWriter, r *http.Request) {
		serveFile(*kernelFile, w)
	})
	http.HandleFunc("/initrd", func(w http.ResponseWriter, r *http.Request) {
		serveFile(*initrdFile, w)
	})
	http.HandleFunc("/cloud-config", func(w http.ResponseWriter, r *http.Request) {
		serveFile(*cloudInit, w)
	})
	http.ListenAndServe(":"+strconv.Itoa(*port), nil)
}

func API(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving boot config for %s", filepath.Base(r.URL.Path))
	http.NotFound(w, r)
	resp := struct {
		K string                 `json:"kernel"`
		I []string               `json:"initrd"`
		C map[string]interface{} `json:"cmdline"`
	}{
		K: fmt.Sprintf("http://%s/kernel", r.Host),
		I: []string{
			fmt.Sprintf("http://%s/initrd", r.Host),
		},
		C: map[string]interface{}{},
	}

	for _, arg := range strings.Split(*kernelCmdline, " ") {
		parts := strings.SplitN(arg, "=", 1)
		if len(parts) == 2 {
			resp.C[parts[0]] = parts[1]
		} else {
			resp.C[parts[0]] = true
		}
	}

	if *cloudInit != "" {
		resp.C["cloud-config-url"] = map[string]string{
			"url": fmt.Sprintf("http://%s/cloud-config", r.Host),
		}
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		panic(err)
	}
}

func serveFile(path string, w io.Writer) {
	log.Printf("Serving file %s", path)
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err = io.Copy(w, f); err != nil {
		panic(err)
	}
}
