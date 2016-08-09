These files are part of the Syslinux project
(http://www.syslinux.org/). They were taken from the 6.03 release, and
get bundled into Pixiecore via go-bindata and `go generate`.

# Build a new version

```
apt-get install -y build-essential nasm uuid-dev
wget https://www.kernel.org/pub/linux/utils/boot/syslinux/Testing/6.04/syslinux-6.04-pre1.tar.gz
cd ~/syslinux-6.04-pre1
make
```

Copy these files into pxelinux

* ./bios/com32/elflink/ldlinux/ldlinux.c32
* ./bios/core/lpxelinux.0

Generate go file:

```
go-bindata -ignore=README.md pxelinux
mv bindata.go pxelinux_autogen.go
```