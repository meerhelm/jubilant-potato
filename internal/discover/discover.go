// Package discover finds RomM servers and SMB file shares on the local
// network by probing every address of the handheld's own /24 subnets.
package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// Host is a machine answering on the probed service.
type Host struct {
	Addr    string // "192.168.1.10:8080"
	Name    string // reverse-DNS name when known
	Version string // service version when known (RomM)
}

// Label is how a host is shown in lists.
func (h Host) Label() string {
	if h.Name != "" {
		return h.Name + " (" + h.Addr + ")"
	}
	return h.Addr
}

const (
	dialTimeout = 400 * time.Millisecond
	parallel    = 96
)

// RomM finds RomM servers on ports 80 and 8080.
func RomM(ctx context.Context) []Host {
	client := &http.Client{Timeout: 2 * time.Second}
	return scan(ctx, []int{8080, 80}, func(ctx context.Context, addr string) (Host, bool) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/heartbeat", nil)
		if err != nil {
			return Host{}, false
		}
		resp, err := client.Do(req)
		if err != nil {
			return Host{}, false
		}
		defer resp.Body.Close()
		var hb struct {
			System struct {
				Version string `json:"VERSION"`
			} `json:"SYSTEM"`
		}
		if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&hb) != nil || hb.System.Version == "" {
			return Host{}, false
		}
		return Host{Addr: addr, Version: hb.System.Version}, true
	})
}

// SMB finds machines accepting SMB connections; Addr is the bare IP.
func SMB(ctx context.Context) []Host {
	hosts := scan(ctx, []int{445}, func(context.Context, string) (Host, bool) { return Host{}, true })
	for i := range hosts {
		hosts[i].Addr, _, _ = net.SplitHostPort(hosts[i].Addr)
	}
	return hosts
}

// scan connects to every port of every subnet address and runs probe on
// the ones that accept, deduplicating by IP (first port wins).
func scan(ctx context.Context, ports []int, probe func(context.Context, string) (Host, bool)) []Host {
	targets := make(chan netip.Addr)
	go func() {
		defer close(targets)
		for _, p := range localSubnets() {
			for a := p.Addr(); p.Contains(a); a = a.Next() {
				select {
				case targets <- a:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	var mu sync.Mutex
	found := map[netip.Addr]Host{}
	var wg sync.WaitGroup
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := net.Dialer{Timeout: dialTimeout}
			for ip := range targets {
				for _, port := range ports {
					addr := netip.AddrPortFrom(ip, uint16(port)).String()
					conn, err := d.DialContext(ctx, "tcp", addr)
					if err != nil {
						continue
					}
					conn.Close()
					h, ok := probe(ctx, addr)
					if !ok {
						continue
					}
					h.Addr = addr
					h.Name = lookupName(ctx, ip)
					mu.Lock()
					found[ip] = h
					mu.Unlock()
					break
				}
			}
		}()
	}
	wg.Wait()

	out := make([]Host, 0, len(found))
	for _, h := range found {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := netip.ParseAddrPort(out[i].Addr)
		b, _ := netip.ParseAddrPort(out[j].Addr)
		return a.Addr().Less(b.Addr())
	})
	return out
}

func lookupName(ctx context.Context, ip netip.Addr) string {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// localSubnets returns the /24 around each private IPv4 address of this
// machine; wider networks are narrowed to keep the scan quick.
func localSubnets() []netip.Prefix {
	var out []netip.Prefix
	seen := map[netip.Prefix]bool{}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP.To4())
			if !ok || !ip.IsPrivate() {
				continue
			}
			bits, _ := ipn.Mask.Size()
			p := netip.PrefixFrom(ip, max(bits, 24)).Masked()
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// Describe summarises what a scan covers, for the UI.
func Describe() string {
	var s []string
	for _, p := range localSubnets() {
		s = append(s, p.String())
	}
	if len(s) == 0 {
		return "no network"
	}
	return fmt.Sprint(strings.Join(s, ", "))
}
