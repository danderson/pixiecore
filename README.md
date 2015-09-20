# Pixiecore, PXE booting for people in a hurry

```
There once was a protocol called PXE,
Whose specification was overly tricksy.
A committee refined it
Into a big Turing tarpit,
And now you're using it to boot your PC.
```

Booting a Linux system over the network is quite tedious. You have to
set up a TFTP server, configure your DHCP server to recognize PXE
clients, and send them the right set of magical options to get them to
boot, often fighting rubbish PXE ROM implementations.

Pixiecore aims to simplify this process, by packing the whole process
into a single binary that can cooperate with your network's existing
DHCP server.

Pixiecore can be used either as a simple "just boot into this OS
image" tool, or as a building block of a machine management system
with its API mode.

## Pixiecore in static mode ("I just want to boot 5 machines")

Run the pixiecore binary, passing it a kernel and initrd, and
optionally some extra kernel commandline arguments.

Here's a couple of examples. If you feel like a screencast instead,
there's a
[very short demo](https://www.youtube.com/watch?v=xjdTOt5YDQM).

### Tiny Core Linux

Tiny Core Linux is a positively tiny distro, clocking in at 24M in the
configuration we'll be using (it can go lower than that). Let's set
ourselves up such that any PXE booting machine on the network boots
into a TinyCore ramdisk:

```shell
# Fetch the kernel and the 3 cpio files that form the filesystem.
wget http://tinycorelinux.net/6.x/x86/release/distribution_files/{vmlinuz64,modules64.gz,core.gz,rootfs.gz}

# In the real world, you would AUTHENTICATE YOUR DOWNLOADS here. TCL sadly
# only distributes images over HTTP, so it's anyone's guess what you
# just downloaded.

# Go!
pixiecore -kernel vmlinuz64 -initrd rootfs.gz,core.gz,modules64.gz
```

That's it. Any machine that tries to netboot on this network will now
boot into a TinyCore ramdisk.

Notice that we passed multiple cpio archives to `-initrd`. All
provided archives will be merged on boot to form the final
ramdisk. This is quite handy for things like providing OEM
configuration without having to respin the upstream initrd image.

### CoreOS

Pixiecore was originally written as a component in an automated
installation system for CoreOS on bare metal. For this example, let's
set up a netboot for the alpha CoreOS release:

```shell
# Grab the PXE images and verify them
wget http://alpha.release.core-os.net/amd64-usr/current/coreos_production_pxe.vmlinuz
wget http://alpha.release.core-os.net/amd64-usr/current/coreos_production_pxe_image.cpio.gz

# In the real world, you would AUTHENTICATE YOUR DOWNLOADS
# here. CoreOS distributes image signatures, but that only really
# helps if you already know the right GPG key.

# Go!
pixiecore -kernel coreos_production_pxe.vmlinuz -initrd coreos_production_pxe_image.cpio.gz --cmdline coreos.autologin
```

Notice that we're passing an extra commandline argument to make CoreOS
automatically log in once it's booted.

## Pixiecore in API mode

Think of Pixiecore in API mode as a "PXE to HTTP" translator. Whenever
Pixiecore sees a machine trying to PXE boot, it will ask a remote HTTP
API (which you implement) what to do. The API server can tell
Pixiecore to ignore the machine, or tell it to boot into a given
kernel/initrd/commandline.

Effectively, Pixiecore in API mode lets you pretend that your machines
speak a simple JSON protocol when trying to netboot. This makes it
_far_ easier to play with netbooting in your own software.

To start Pixiecore in API mode, pass it the HTTP API endpoint through
the `-api` flag. The endpoint you provide must implement the Pixiecore
boot API, as described in the [API spec](README.api.md).

You can find a sample API server implementation in the `example`
subdirectory. The code is not production-grade, but gives a short
illustration of how the protocol works by reimplementing a subset of
Pixiecore's static mode as an API server.

## Running in Docker

Pixiecore is available as a Docker image called
`danderson/pixiecore`. It's an automatic Docker Hub build that tracks
the repository.

Because Pixiecore needs to listen for DHCP traffic, it has to run with
the host network stack.

```shell
sudo docker run -v .:/image --net=host danderson/pixiecore -kernel /image/coreos_production_pxe.vmlinuz -initrd /image/coreos_prodeuction_pxe_image.cpio.gz
```

## How it works

Pixiecore implements four different, but related protocols in one
binary, which together can take a PXE ROM from nothing to booting
Linux. They are: ProxyDHCP, PXE, TFTP, and HTTP. Let's walk through
the boot process for a PXE ROM.

### DHCP/ProxyDHCP

The first thing a PXE ROM does is request a configuration through
DHCP, waiting for a DHCP reply that includes PXE vendor options. The
normal way of providing these options is to edit your DHCP server's
configuration to provide them to clients that identify themselves as
PXE clients. Unfortunately, reconfiguring your network's DHCP server
is tedious at best, and impossible if you DHCP server is built into a
consumer router, or managed by someone else.

Pixiecore instead uses a feature of the PXE specification called
_ProxyDHCP_. As you might guess from the name, ProxyDHCP is not a
proxy at all (yeah, the PXE spec is like that), but a second DHCP
server that only provides PXE configuration.

When the PXE ROM sends out a `DHCPDISCOVER`, it gets two replies back:
one containing network configuration from the primary DHCP server, and
one containing only PXE DHCP options from the ProxyDHCP server. The
PXE firmware combines the two, and continues as if the primary server
had provided all the configuration.

### PXE

In theory, you'd expect the ProxyDHCP server to just provide a TFTP
server IP and a filename to the PXE firmware, and it would proceed to
download and boot that just like the BOOTP of old.

Sadly, the average quality of PXE ROM implementations is abysmal, and
many of them fail to chainload correctly if you try to do this from a
ProxyDHCP server.

So, instead, we make use of the spec's "PXE menu" functionality, which
lets you tell the PXE firmware to display a boot menu. Just like
everything else in PXE, this is quite brittle, so nobody actually uses
it to display menus - instead, they just push a more fully featured
bootloader over PXE, and let that bootloader do the fancy work.

However, PXE menus seem to work reliably when combined with
ProxyDHCP... And the PXE configuration can provide a timeout after
which the first menu entry is booted... And that timeout can be set to
zero.

So, we can just provide a single-entry menu, with a zero timeout, and
chainload that way! But wait, there's more terribleness. PXE menu
entries don't just list a TFTP server and file to load, because that
would be too simple. Instead, each menu entry maps to a "Boot Server
Type", and yet another DHCP option maps that boot server type to a set
of IP addresses.

Those IP addresses aren't TFTP servers, but PXE boot servers. PXE boot
servers listen on port 4011. They use the DHCP packet format, but only
as a way of conveying a DHCP option that says "please tell me how to
boot the following Boot Server Type". It's quite possibly the least
efficient protocol encoding ever devised.

At long last, when the PXE server receives that request, it can reply
with a BOOTP-ish packet that specified next-server and a filename. And
_those_ are, at long last, TFTP.

### TFTP

After navigating the eldritch horror of PXE, TFTP is a breath of fresh
air. It is indeed a trivial protocol for transferring files. I have
found some PXE ROMs that manage to add unnecessary complexity even to
that, but by and large, this step is straightforward.

However, TFTP is quite slow, because it doesn't support transfer
windows (well, it does, but it's an extension defined in an RFC
published in 2015, so guess how many PXE ROMs implement it...). As a
result, you must pay one round-trip per ~1500 bytes transferred, and
even on a gigabit network, that slows things down.

Given that some netboot images are quite large (CoreOS clocks in at
almost 200MB), what we really want is to switch to a more efficient
protocol. That's where PXELINUX comes in.

PXELINUX is a small bootloader that knows how to boot Linux kernels,
and it comes in a variant that can speak HTTP. PXELINUX is 90kB, which
even over TFTP is very fast to transfer.

Thus, Pixiecore uses TFTP only to transfer PXELINUX, and from there
steers it to HTTP for the rest of the loading process.

### HTTP

We've finally crawled our way up to the late nineties - we can speak
HTTP! Pixiecore's HTTP server is wonderfully familiar and normal. It
just serves up a support file that PXELINUX needs (`ldlinux.c32`), a
trivial PXELINUX configuration telling it to boot a Linux kernel, and
the user-provided kernel and initrd files.

PXELINUX grabs all of that, and finally, Linux boots.

### Recap

This is what the whole boot process looks like on the wire.

#### Dramatis Personae¦

- **PXE ROM**, a brittle firmware burned into the network card.
- **DHCP server**, a plain old DHCP server providing network configuration.
- **Pixieboot**, the Hero and server of ProxyDHCP, PXE, TFTP and HTTP.
- **PXELINUX**, an open source bootloader of the [Syslinux project](http://www.syslinux.org).

#### Timeline

- PXE ROM starts, broadcasts `DHCPDISCOVER`.
- DHCP server responds with a `DHCPOFFER` containing network configs.
- Pixiecore's ProxyDHCP server responds with a `DHCPOFFER` containing a PXE boot menu.
- PXE ROM does a `DHCPREQUEST`/`DHCPACK` exchange with the DHCP server to get a network configuration.
- PXE ROM processes the PXE boot menu, decides to boot menu entry 0.
- PXE ROM sends a `DHCPREQUEST` to Pixiecore's PXE server, asking for a boot file.
- Pixiecore's PXE server responds with a `DHCPACK` listing a TFTP
  server, a boot filename, and a PXELINUX vendor option to make it use
  HTTP.
- PXE ROM downloads PXELINUX from Pixiecore's TFTP server, and hands off to PXELINUX.
- PXELINUX fetches its configuration from Pixiecore's HTTP server.
- PXELINUX fetches a kernel and ramdisk from Pixiecore's HTTP server, and boots Linux.
