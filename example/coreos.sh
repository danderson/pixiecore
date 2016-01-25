#!/bin/bash
#This will boot, but not install coreos, and leave the PC with an open terminal
mkdir coreos
rm coreos/coreos_production_pxe.vmlinuz
rm coreos/coreos_production_pxe_image.cpio.gz
wget -O coreos/coreos.vmlinux http://alpha.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz
wget -O coreos/coreos.gz http://alpha.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz
pixiecore -kernel coreos_production_pxe.vmlinuz -initrd coreos_production_pxe_image.cpio.gz --cmdline coreos.autologin
