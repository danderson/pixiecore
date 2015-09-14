package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

//go:generate go-bindata -o pxelinux_autogen.go -prefix=pxelinux pxelinux

var (
	portPXE  = flag.Int("pxe-port", 67, "Port to listen on for PXE DHCP requests")
	portTFTP = flag.Int("tftp-port", 69, "Port to listen on for TFTP requests")
	portHTTP = flag.Int("http-port", 70, "Port to listen on for HTTP requests")

	kernelFile    = flag.String("kernel", "vmlinuz", "Path to the linux kernel file to boot")
	initrdFile    = flag.String("initrd", "initrd", "Path to the initrd file to boot")
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

	go func() {
		log.Fatalln(ServePXE(*portPXE, *portHTTP))
	}()
	go func() {
		log.Fatalln(ServeTFTP(*portTFTP, pxelinux))
	}()
	go func() {
		log.Fatalln(ServeHTTP(*portHTTP, ldlinux, *kernelFile, *initrdFile, *kernelCmdline))
	}()
	RecordLogs(*debug)
}
