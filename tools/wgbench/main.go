// wgbench: WireGuard throughput test.
//   wgbench server <listen addr>                 (on the stick, inside the tunnel)
//   wgbench client <conf> <server ip:port> <secs> (on the VM: userspace netstack WireGuard peer)
// conf lines: priv=<b64> peer=<b64 pub> endpoint=<ip:port> local=<tunnel ip>
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func b64hex(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	die(err)
	return hex.EncodeToString(b)
}

func server(addr string) {
	ln, err := net.Listen("tcp", addr)
	die(err)
	buf := make([]byte, 64*1024)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			mode := make([]byte, 1)
			if _, err := io.ReadFull(c, mode); err != nil {
				return
			}
			if mode[0] == 'U' { // client uploads, we discard
				io.Copy(io.Discard, c)
				return
			}
			end := time.Now().Add(time.Duration(mode[0]) * time.Second) // 'D' path: mode byte = seconds
			for time.Now().Before(end) {
				if _, err := c.Write(buf); err != nil {
					return
				}
			}
		}(c)
	}
}

func client(conf, srv string, secs int) {
	kv := map[string]string{}
	f, err := os.Open(conf)
	die(err)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			kv[k] = v
		}
	}
	tdev, tnet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr(kv["local"])}, []netip.Addr{}, 1420)
	die(err)
	dev := device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, ""))
	die(dev.IpcSet(fmt.Sprintf("private_key=%s\npublic_key=%s\nendpoint=%s\nallowed_ip=0.0.0.0/0\npersistent_keepalive_interval=25\n",
		b64hex(kv["priv"]), b64hex(kv["peer"]), kv["endpoint"])))
	die(dev.Up())
	run := func(name string, upload bool) {
		c, err := tnet.Dial("tcp", srv)
		die(err)
		defer c.Close()
		buf := make([]byte, 64*1024)
		var n int64
		start := time.Now()
		if upload {
			c.Write([]byte{'U'})
			for time.Since(start) < time.Duration(secs)*time.Second {
				w, err := c.Write(buf)
				n += int64(w)
				if err != nil {
					break
				}
			}
		} else {
			c.Write([]byte{byte(secs)})
			for {
				r, err := c.Read(buf)
				n += int64(r)
				if err != nil {
					break
				}
			}
		}
		el := time.Since(start).Seconds()
		fmt.Printf("%-28s %6.1f Mbit/s  (%d MB in %.1f s)\n", name, float64(n)*8/el/1e6, n>>20, el)
	}
	// warm up the handshake
	for i := 0; i < 20; i++ {
		if c, err := tnet.Dial("tcp", srv); err == nil {
			c.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	run("VM -> stick (stick decrypts)", true)
	run("stick -> VM (stick encrypts)", false)
}

func main() {
	switch os.Args[1] {
	case "server":
		server(os.Args[2])
	case "client":
		s, _ := strconv.Atoi(os.Args[4])
		client(os.Args[2], os.Args[3], s)
	}
}
