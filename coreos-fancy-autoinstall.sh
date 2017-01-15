#!/bin/bash
#This script makes a critical assumption:  It assumes that you are the end-user (the person who will ssh into the servers in question), and you'd like your ~/.ssh/id_rsa.pub used as the key for the coreos install.  I'm going to try it first without a cloud-config, because I think coreos will let us get away with just having the SSH key in -cmdline.  We shall see.
#2nd assumption: You'd like GVM installed on your machine
#3rd assumption: you're using an amd64 box
#4th assumption: you're OK with installing coreos-cloudinit via go get github.com/coreos/coreos/cloudinit
#5th assumption: you're using Debian or Ubuntu as your operating system.  Linux mint and users of most debian-derived distros should be safe, too.  Arch users should be able to write a better script, since they can install Arch.  
#see also: https://github.com/coreos/coreos-baremetal
export PUBKEY=$(cat ~/.ssh/id_rsa.pub)
rm -rf coreos_fancy
mkdir coreos_fancy
wget -O coreos_fancy/coreos_production_pxe.vmlinuz http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz
wget -O coreos_fancy/coreos_production_pxe.vmlinuz.sig http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz.sig
wget -O coreos_fancy/coreos_production_pxe_image.cpio.gz http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz
wget -O coreos_fancy/coreos_production_pxe_image.cpio.gz.sig http://beta.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz.sig
gpg --verify coreos/coreos_production_pxe.vmlinuz.sig
gpg --verify coreos/coreos_production_pxe_image.cpio.gz.sig
# the commands are extremely useful, as is line 6.  Line 6 grabs your ssh key (you can use this technique to put any text file into an environment variable), and on line 16 >> tells the grabbed ssh key to be appended to the last line of a cloud-config.yml file, which we will have set up so that it expects the last line to be an SSH public key.
#one arrow to make a new file
echo "#cloud-config" > cloud-config.yml 
#two arrows to send to the last line of an existing file
echo "ssh_authorized_keys:" >> cloud-config.yml 
echo "  - $PUBKEY" >> cloud-config.yml
#installs GVM-- if you even sorta use go, gvm is a lifesaver par excellence.
bash < <(curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer)
source ~/.gvm/scripts/gvm
gvm install go1.5.3 -B
gvm use go1.5.3 --default
go get github.com/coreos/coreos-cloudinit
#the command below will ensure that your cloud-config.yml checks out before you try and start the system with it, and find out you've missed a space or something three days after you began, and that's why you can't ssh into the server.
coreos-cloudinit -validate --from-file=cloud-config.yml
sudo apt-get install -y -qq moreutils
go get github.com/mholt/caddy
caddy > caddy.log
sudo ifdata -pa eth0 > eth0
export IPADDR=$(cat eth0)
sudo pixiecore -kernel=coreos_production_pxe.vmlinuz -initrd=coreos_production_pxe_image.cpio.gz -cmdline=http://$IPADDR:2015/cloud-config.yml
