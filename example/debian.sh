#!/bin/bash
#This script will delete any existing initrd/kernel for debian in /examples/debian and then download today's latest & greatest initrd & debian.  Lastly, it will use a minimal debian preseed to autoinstall on as many hosts as you feel like rebooting.
rm debian/linux
rm debian/initrd.gz
wget http://ftp.debian.org/debian/dists/jessie/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux
wget http://ftp.debian.org/debian/dists/jessie/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz
pixiecore -initrd debian/initrd.gz -kernel debian/linux -cmdline="auto=true url=https://www.debian.org/releases/jessie/example-preseed.txt"
