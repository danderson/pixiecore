#!/bin/bash
#This script will delete any existing initrd/kernel for ubuntu in /examples/ubuntu and then download today's latest & greatest initrd & ubuntu.  Lastly, it will use a minimal Ubuntu preseed to autoinstall on as many hosts as you feel like rebooting.
mkdir ubuntu
rm ubuntu/initrd.gz
rm ubuntu/linux
wget -O ubuntu/initrd.gz http://archive.ubuntu.com/ubuntu/dists/trusty-updates/main/installer-amd64/current/images/wily-netboot/ubuntu-installer/amd64/initrd.gz
wget -O ubuntu/linux http://archive.ubuntu.com/ubuntu/dists/trusty-updates/main/installer-amd64/current/images/wily-netboot/ubuntu-installer/amd64/linux
sudo pixiecore -kernel=ubuntu/linux -initrd=ubuntu/initrd.gz -cmdline="auto url=http://preseed.panticz.de/preseed/ubuntu-minimal.seed"

