package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

//go:generate go-bindata -o pxelinux_autogen.go -prefix=pxelinux -ignore=README.md pxelinux

var (
	// I'm sort of giving you the option to change these ports here,
	// but all of them except the HTTP port are hardcoded in the PXE
	// option ROM, so it's pretty pointless unless you'd playing
	// packet rewriting tricks or doing simulations with packet
	// generators.
	portDHCP = flag.Int("port-dhcp", 67, "Port to listen on for DHCP requests")
	portPXE  = flag.Int("port-pxe", 4011, "Port to listen on for PXE requests")
	portTFTP = flag.Int("port-tftp", 69, "Port to listen on for TFTP requests")
	portHTTP = flag.Int("port-http", 70, "Port to listen on for HTTP requests")

	kernelFile    = flag.String("kernel", "", "Path to the linux kernel file to boot")
	initrdFile    = flag.String("initrd", "", "Comma-separated list of initrds to pass to the kernel")
	kernelCmdline = flag.String("cmdline", "", "Additional arguments for the kernel commandline")

	debug = flag.Bool("debug", false, "Log more things that aren't directly related to booting a recognized client")
)

func main() {
	flag.Parse()

	if *kernelFile == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nERROR: Please provide a linux kernel to boot with -kernel.\n")
		os.Exit(1)
	}
	if *initrdFile == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nERROR: Please provide an initrd to boot with -initrd.\n")
		os.Exit(1)
	}

	pxelinux, err := Asset("lpxelinux.0")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ldlinux, err := Asset("ldlinux.c32")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	initrds := strings.Split(*initrdFile, ",")

	go func() {
		log.Fatalln(ServeProxyDHCP(*portDHCP))
	}()
	go func() {
		log.Fatalln(ServePXE(*portPXE, *portHTTP))
	}()
	go func() {
		log.Fatalln(ServeTFTP(*portTFTP, pxelinux))
	}()
	go func() {
		log.Fatalln(ServeHTTP(*portHTTP, ldlinux, *kernelFile, initrds, *kernelCmdline))
	}()
	RecordLogs(*debug)
}
