package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
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

type spec struct {
	BootSpec
	cmdMap map[string]interface{}
}

// A Booter tells Pixiecore whether/how to boot machines.
type Booter interface {
	// The given MAC address is attempting to netboot. Should
	// Pixiecore offer to help?
	ShouldBoot(addr net.HardwareAddr) error
	// The given MAC address is now running a bootloader, and it wants
	// to know what it should boot. Returning an error here will cause
	// the PXE boot process to abort (i.e. the machine will reboot and
	// start again at ShouldBoot).
	BootSpec(addr net.HardwareAddr, fileURLPrefix string) (*BootSpec, error)
	// Get the contents of a blob mentioned in a previously issued
	// BootSpec. Additionally returns a pretty name for the blob for
	// logging purposes.
	File(id string) (io.ReadCloser, string, error)
}

// RemoteBooter gets a BootSpec from a remote server over HTTP.
//
// The API is described in README.api.md
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

func (b *remoteBooter) getAPIResponse(hw net.HardwareAddr) (io.ReadCloser, error) {
	reqURL := fmt.Sprintf("%s/boot/%s", b.urlPrefix, hw)
	resp, err := b.client.Get(reqURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", reqURL, http.StatusText(resp.StatusCode))
	}

	return resp.Body, nil
}

func (b *remoteBooter) ShouldBoot(hw net.HardwareAddr) error {
	r, err := b.getAPIResponse(hw)
	if r != nil {
		r.Close()
	}
	return err
}

func (b *remoteBooter) BootSpec(hw net.HardwareAddr, fileURLPrefix string) (*BootSpec, error) {
	body, err := b.getAPIResponse(hw)
	defer body.Close()
	if err != nil {
		return nil, err
	}

	r := struct {
		Kernel  string      `json:"kernel"`
		Initrd  []string    `json:"initrd"`
		Cmdline interface{} `json:"cmdline"`
	}{}
	if err = json.NewDecoder(body).Decode(&r); err != nil {
		return nil, err
	}

	// Check that the API server gave us absolute URLs for everything
	if err = isURLAbsolute(r.Kernel); err != nil {
		return nil, err
	}
	for _, img := range r.Initrd {
		if err = isURLAbsolute(img); err != nil {
			return nil, err
		}
	}

	var ret BootSpec
	if ret.Kernel, err = b.signURL(r.Kernel, fileURLPrefix); err != nil {
		return nil, err
	}
	for _, img := range r.Initrd {
		initrd, err := b.signURL(img, fileURLPrefix)
		if err != nil {
			return nil, err
		}
		ret.Initrd = append(ret.Initrd, initrd)
	}

	switch c := r.Cmdline.(type) {
	case string:
		ret.Cmdline = c
	case map[string]interface{}:
		ret.Cmdline, err = b.constructCmdline(c, fileURLPrefix)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("API server returned unknown type %T for kernel cmdline", r.Cmdline)
	}

	return &ret, nil
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

func (b *remoteBooter) constructCmdline(m map[string]interface{}, fileURLPrefix string) (string, error) {
	ret := make([]string, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	sort.Strings(ret)
	for i, k := range ret {
		switch v := m[k].(type) {
		case bool:
		case string:
			ret[i] = fmt.Sprintf("%s=%s", k, v)
		case map[string]interface{}:
			urlStr, ok := v["url"].(string)
			if !ok {
				return "", fmt.Errorf("cmdline key %q has object value with no 'url' attribute", k)
			}
			if err := isURLAbsolute(urlStr); err != nil {
				return "", fmt.Errorf("invalid url for cmdline key %q: %s", k, err)
			}
			encoded, err := b.signURL(urlStr, fileURLPrefix)
			if err != nil {
				return "", err
			}
			ret[i] = fmt.Sprintf("%s=%s", k, encoded)
		default:
			return "", fmt.Errorf("unsupported value kind %T for cmdline key %q", m[k], k)
		}
	}
	return strings.Join(ret, " "), nil
}

func (b *remoteBooter) signURL(u, fileURLPrefix string) (string, error) {
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
	return fileURLPrefix + base64.URLEncoding.EncodeToString(out), nil
}

func (b *remoteBooter) getURL(signedStr string) (string, error) {
	signed, err := base64.URLEncoding.DecodeString(signedStr)
	if err != nil {
		return "", err
	}
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

// StaticBooter boots all machines with local files.
func StaticBooter(kernelPath string, initrdPaths []string, cmdline string) Booter {
	return &staticBooter{
		kernelPath:  kernelPath,
		initrdPaths: initrdPaths,
		cmdline:     cmdline,
	}
}

type staticBooter struct {
	kernelPath  string
	initrdPaths []string
	cmdline     string
}

func (b *staticBooter) ShouldBoot(net.HardwareAddr) error {
	return nil
}

func (b *staticBooter) BootSpec(unused net.HardwareAddr, prefix string) (*BootSpec, error) {
	ret := &BootSpec{
		Kernel:  prefix + "kernel",
		Cmdline: b.cmdline,
	}
	for i := range b.initrdPaths {
		ret.Initrd = append(ret.Initrd, fmt.Sprintf("%s%d", prefix, i))
	}
	return ret, nil
}

func (b staticBooter) File(id string) (io.ReadCloser, string, error) {
	if id == "kernel" {
		fmt.Println(b.kernelPath)
		f, err := os.Open(b.kernelPath)
		return f, "kernel", err
	} else if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(b.initrdPaths) {
		f, err := os.Open(b.initrdPaths[i])
		return f, "initrd." + id, err
	}
	return nil, "", fmt.Errorf("no file with ID %q", id)
}

func isURLAbsolute(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("%q is not an URL", urlStr)
	}
	if !u.IsAbs() {
		return fmt.Errorf("URL %q is not absolute", urlStr)
	}
	return nil
}
