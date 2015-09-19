package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

// A BootSpec identifies a kernel, kernel commandline, and set of initrds to boot on a machine.
//
// Kernel and Initrds are opaque reference strings provided by a
// Booter. When we need to get the associated bytes, we pass the
// opaque reference back into Booter.File(). The bytes have no other
// significance beyond that. They also do not need to be
// human-readable.
type BootSpec struct {
	Kernel  string
	Initrd  []string
	Cmdline string
}

type Booter interface {
	// The given MAC address is attempting to netboot. Should
	// Pixiecore offer to help?
	ShouldBoot(net.HardwareAddr) error
	// The given MAC address is now running a bootloader, and it wants
	// to know what it should boot. Returning an error here will cause
	// the PXE boot process to abort (i.e. the machine will reboot and
	// start again at ShouldBoot).
	BootSpec(net.HardwareAddr) (*BootSpec, error)
	// Get the contents of a blob mentioned in a previously issued
	// BootSpec. Additionally returns a pretty name for the blob for
	// logging purposes.
	File(id string) (io.ReadCloser, string, error)
}

func RemoteBooter(url string, timeout time.Duration) (Booter, error) {
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	ret := &remoteBooter{
		client:    &http.Client{Timeout: timeout},
		urlPrefix: url + "v1",
	}
	if _, err := io.ReadFull(rand.Reader, ret.key[:]); err != nil {
		return nil, fmt.Errorf("failed to get randomness for signing key: %s", err)
	}

	return ret, nil
}

type remoteBooter struct {
	client    *http.Client
	urlPrefix string
	key       [32]byte
}

func (b *remoteBooter) getSpec(hw net.HardwareAddr) (string, []string, string, error) {
	reqURL := fmt.Sprintf("%s/boot/%s", b.urlPrefix, hw)
	resp, err := b.client.Get(reqURL)
	if err != nil {
		return "", nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, "", fmt.Errorf("%s: %s", reqURL, http.StatusText(resp.StatusCode))
	}

	r := struct {
		Kernel  string   `json:"kernel"`
		Initrd  []string `json:"initrd"`
		Cmdline string   `json:"cmdline"`
	}{}
	if err = json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", nil, "", fmt.Errorf("non-json response from %s: %s", reqURL, err)
	}

	// Check that the API server gave us absolute URLs for everything
	u, err := url.Parse(r.Kernel)
	if err != nil {
		return "", nil, "", fmt.Errorf("non-url %q provided by %s for kernel: %s", r.Kernel, reqURL, err)
	}
	if !u.IsAbs() {
		return "", nil, "", fmt.Errorf("kernel URL %q provided by %s is not absolute", u, reqURL)
	}

	for _, img := range r.Initrd {
		u, err := url.Parse(img)
		if err != nil {
			return "", nil, "", fmt.Errorf("non-url %q provided by %s for initrd: %s", img, reqURL, err)
		}
		if !u.IsAbs() {
			return "", nil, "", fmt.Errorf("initrd URL %q provided by %s is not absolute", img, reqURL)
		}
	}

	return r.Kernel, r.Initrd, r.Cmdline, nil
}

func (b *remoteBooter) ShouldBoot(hw net.HardwareAddr) error {
	_, _, _, err := b.getSpec(hw)
	return err
}

func (b *remoteBooter) BootSpec(hw net.HardwareAddr) (*BootSpec, error) {
	kernel, initrds, cmdline, err := b.getSpec(hw)
	if err != nil {
		return nil, err
	}

	ret := &BootSpec{
		Cmdline: cmdline,
	}
	ret.Kernel, err = b.signURL(kernel)
	if err != nil {
		return nil, err
	}
	for _, img := range initrds {
		initrd, err := b.signURL(img)
		if err != nil {
			return nil, err
		}
		ret.Initrd = append(ret.Initrd, initrd)
	}

	return ret, nil
}

func (b *remoteBooter) File(id string) (io.ReadCloser, string, error) {
	u, err := b.getURL(id)
	if err != nil {
		return nil, "", err
	}
	// Can't use the handbuilt client we have, it times out too
	// aggressively. Need to work on that.
	resp, err := http.Get(u)
	if err != nil {
		return nil, "", err
	}
	return resp.Body, u, nil
}

func (b *remoteBooter) signURL(u string) (string, error) {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("could not read randomness for signing nonce: %s", err)
	}

	out := nonce[:]

	// Secretbox is authenticated encryption. In theory we only need
	// symmetric authentication, but secretbox is stupidly simple to
	// use and hard to get wrong, and the encryption overhead should
	// be tiny for such a small URL unless you're trying to
	// simultaneously netboot a million machines. This is one case
	// where convenience and certainty that you got it right trumps
	// pure efficiency.
	out = secretbox.Seal(out, []byte(u), &nonce, &b.key)
	return string(out), nil
}

func (b *remoteBooter) getURL(signed string) (string, error) {
	if len(signed) < 24 {
		return "", errors.New("signed blob too short to be valid")
	}

	var nonce [24]byte
	copy(nonce[:], signed)
	out, ok := secretbox.Open(nil, []byte(signed[24:]), &nonce, &b.key)
	if !ok {
		return "", errors.New("signature verification failed")
	}

	return string(out), nil
}

func StaticBooter(kernelPath string, initrdPaths []string, cmdline string) Booter {
	ret := &staticBooter{
		kernelPath:  kernelPath,
		initrdPaths: initrdPaths,
		spec: BootSpec{
			Kernel:  "kernel",
			Cmdline: cmdline,
		},
	}

	for i := range initrdPaths {
		ret.spec.Initrd = append(ret.spec.Initrd, strconv.Itoa(i))
	}

	return ret
}

type staticBooter struct {
	kernelPath  string
	initrdPaths []string
	spec        BootSpec
}

func (b *staticBooter) ShouldBoot(net.HardwareAddr) error {
	return nil
}

func (b *staticBooter) BootSpec(net.HardwareAddr) (*BootSpec, error) {
	return &BootSpec{
		Kernel:  b.spec.Kernel,
		Initrd:  append([]string(nil), b.spec.Initrd...),
		Cmdline: b.spec.Cmdline,
	}, nil
}

func (b staticBooter) File(id string) (io.ReadCloser, string, error) {
	if id == "kernel" {
		f, err := os.Open(b.kernelPath)
		return f, "kernel", err
	} else if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(b.initrdPaths) {
		f, err := os.Open(b.initrdPaths[i])
		return f, "initrd." + id, err
	}
	return nil, "", fmt.Errorf("no file with ID %q", id)
}
