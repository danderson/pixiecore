package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

var (
	portPXE      = flag.Int("pxe-port", 67, "Port to listen on for PXE DHCP requests")
	portTFTP     = flag.Int("tftp-port", 69, "Port to listen on for TFTP requests")
	portHTTP     = flag.Int("http-port", 70, "Port to listen on for HTTP requests")
	pxelinuxFile = flag.String("pxelinux", "lpxelinux.0", "Path to the HTTP-enabled pxelinux image (usually called lpxelinux.0)")
	ldlinuxFile  = flag.String("ldlinux32", "ldlinux.c32", "Path to syslinux's ldlinux.c32")
	debug        = flag.Bool("debug", false, "Log more things that aren't directly related to booting a recognized client")
)

func main() {
	flag.Parse()
	pxelinux, err := ioutil.ReadFile(*pxelinuxFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ldlinux, err := ioutil.ReadFile(*ldlinuxFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	go func() {
		log.Fatalln(ServePXE(*portPXE, *portHTTP))
	}()
	go func() {
		log.Fatalln(ServeTFTP(*portTFTP, pxelinux))
	}()
	go func() {
		log.Fatalln(ServeHTTP(*portHTTP, ldlinux))
	}()
	RecordLogs(*debug)
}
