#!/bin/bash
#This is a sample cloud-config-using rancheros PXE.  Thing is, I've had a tough time with the cloud-config, so that's left as an exercise to the user, here's the [example](http://docs.rancher.com/os/cloud-config/) page: 
rm -rf rancheros
mkdir rancheros
wget -O rancheros/initrd https://releases.rancher.com/os/latest/initrd
wget -O rancheros/kernel https://releases.rancher.com/os/latest/vmlinuz
cd rancheros
nohup sudo python -m SimpleHTTPServer 850 > cloud-config.log
echo "what IP address to bind pixiecore to, wise user?"
read $IP_ADDR
pixiecore -kernel=rancheros/kernel -initrd=rancheros/initrd -cmdline=rancher.cloud_init.datasources=[url:http://$IP_ADDR:850/cloud-config-master.yml]
