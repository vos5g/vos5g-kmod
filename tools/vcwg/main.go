// vcwg: minimal WireGuard configuration tool for the VOS stick (no wg binary there).
//   vcwg genkey                      -> private key (base64)
//   vcwg pubkey <priv>               -> public key
//   vcwg set <iface> <priv> <listenport> <peerpub> <endpoint|-> <allowedip,...>
//   vcwg show <iface>
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: vcwg genkey | pubkey <priv> | set <iface> <priv> <port> <peerpub> <endpoint|-> <allowedips> | show <iface>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "add": // create a kernel WireGuard interface (iproute2 here predates wireguard)
		die(netlink.LinkAdd(&netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: os.Args[2], MTU: 1420}, LinkType: "wireguard"}))
	case "del":
		l, err := netlink.LinkByName(os.Args[2])
		die(err)
		die(netlink.LinkDel(l))
	case "genkey":
		k, err := wgtypes.GeneratePrivateKey()
		die(err)
		fmt.Println(k.String())
	case "pubkey":
		k, err := wgtypes.ParseKey(os.Args[2])
		die(err)
		fmt.Println(k.PublicKey().String())
	case "set":
		priv, err := wgtypes.ParseKey(os.Args[3])
		die(err)
		port, _ := strconv.Atoi(os.Args[4])
		pub, err := wgtypes.ParseKey(os.Args[5])
		die(err)
		peer := wgtypes.PeerConfig{PublicKey: pub, ReplaceAllowedIPs: true}
		if os.Args[6] != "-" {
			ep, err := net.ResolveUDPAddr("udp", os.Args[6])
			die(err)
			peer.Endpoint = ep
			ka := 25 * time.Second
			peer.PersistentKeepaliveInterval = &ka
		}
		for _, c := range strings.Split(os.Args[7], ",") {
			_, n, err := net.ParseCIDR(c)
			die(err)
			peer.AllowedIPs = append(peer.AllowedIPs, *n)
		}
		cl, err := wgctrl.New()
		die(err)
		die(cl.ConfigureDevice(os.Args[2], wgtypes.Config{PrivateKey: &priv, ListenPort: &port, ReplacePeers: true, Peers: []wgtypes.PeerConfig{peer}}))
	case "show":
		cl, err := wgctrl.New()
		die(err)
		d, err := cl.Device(os.Args[2])
		die(err)
		fmt.Printf("interface %s type %s port %d\n", d.Name, d.Type, d.ListenPort)
		for _, p := range d.Peers {
			fmt.Printf("peer %s endpoint %v handshake %v rx %d tx %d\n", p.PublicKey, p.Endpoint, p.LastHandshakeTime.Format(time.RFC3339), p.ReceiveBytes, p.TransmitBytes)
		}
	}
}
