package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

//go:generate go-bindata -o pxelinux_autogen.go -prefix=pxelinux pxelinux

var (
	// I'm sort of giving you the option to change these ports here,
	// but all of them except the HTTP port are hardcoded in the PXE
	// option ROM, so it's pretty pointless unless you'd playing
	// packet rewriting tricks or doing simulations with packet
	// generators.
	portDHCP = flag.Int("dhcp-port", 67, "Port to listen on for DHCP requests")
	portPXE  = flag.Int("pxe-port", 4011, "Port to listen on for PXE requests")
	portTFTP = flag.Int("tftp-port", 69, "Port to listen on for TFTP requests")
	portHTTP = flag.Int("http-port", 70, "Port to listen on for HTTP requests")

	kernelFile    = flag.String("kernel", "vmlinuz", "Path to the linux kernel file to boot")
	initrdFile    = flag.String("initrd", "initrd", "Comma-separated list of initrds to pass to the kernel")
	kernelCmdline = flag.String("cmdline", "", "Additional arguments for the kernel commandline")

	debug = flag.Bool("debug", false, "Log more things that aren't directly related to booting a recognized client")
)

func main() {
	flag.Parse()

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
