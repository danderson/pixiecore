#!/bin/bash
#This will boot, but not install coreos, and leave the PC with an open terminal
mkdir coreos
rm coreos/coreos_production_pxe.vmlinuz
rm coreos/coreos_production_pxe_image.cpio.gz
wget -O coreos/coreos_production_pxe.vmlinuz http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz
wget -O coreos/coreos_production_pxe.vmlinuz.sig http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz.sig
wget -O coreos/coreos_production_pxe_image.cpio.gz http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz
wget -O coreos/coreos_production_pxe_image.cpio.gz.sig http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz.sig
gpg --verify coreos/coreos_production_pxe.vmlinuz.sig
gpg --verify coreos/coreos_production_pxe_image.cpio.gz.sig
pixiecore -kernel coreos_production_pxe.vmlinuz -initrd coreos_production_pxe_image.cpio.gz --cmdline coreos.autologin
