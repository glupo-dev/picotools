package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cleanHost accepts a name, an IP or a whole address like "https://site.com/page"
// and returns only the host part.
func cleanHost(input string) string {
	s := strings.TrimSpace(input)
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			return u.Hostname()
		}
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSuffix(s, ".")
}

// resolve finds the addresses of host, with a readable error if the name does not exist.
func resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, fmt.Errorf("address not found: %s", host)
		}
		return nil, err
	}
	return ips, nil
}

// ---- Ping ----

// pingTime finds the time in a reply line, in any language:
// "time=12.3 ms" (Mac), "durata=12ms" or "tempo<1ms" (Windows).
var pingTime = regexp.MustCompile(`[=<]\s*([0-9]+(?:[.,][0-9]+)?)\s*ms`)

const pingCount = 4

// ping sends 4 pings, one per second, and shows each attempt and a summary.
// It uses the system ping, which works without administrator rights on Mac and Windows.
func ping(input string, show func([]Result)) ([]Result, error) {
	host := cleanHost(input)
	if host == "" {
		return nil, errors.New("please type an address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	ips, err := resolve(ctx, host)
	cancel()
	if err != nil {
		return nil, err
	}
	ip := ips[0].String()

	results := []Result{{Name: "IP address", Value: ip}}
	show(slices.Clone(results))
	var times []float64
	for i := 1; i <= pingCount; i++ {
		start := time.Now()
		value := "no answer"
		if t, ok := pingOnce(ip); ok {
			times = append(times, t)
			value = fmt.Sprintf("%.1f ms", t)
		}
		results = append(results, Result{Name: fmt.Sprintf("Attempt %d", i), Value: value})
		show(slices.Clone(results))
		if i < pingCount {
			time.Sleep(time.Second - time.Since(start)) // one ping per second, like the ping command
		}
	}

	lost := pingCount - len(times)
	summary := fmt.Sprintf("%d sent, %d received, %d%% lost", pingCount, len(times), lost*100/pingCount)
	if len(times) == 0 {
		summary += " · the host is down or does not reply to ping"
	}
	results = append(results, Result{Name: "Packets", Value: summary})
	if len(times) > 0 {
		sum := 0.0
		for _, t := range times {
			sum += t
		}
		results = append(results, Result{Name: "Min / Avg / Max", Value: fmt.Sprintf("%.1f / %.1f / %.1f ms", slices.Min(times), sum/float64(len(times)), slices.Max(times))})
	}
	return results, nil
}

// pingOnce sends a single ping and returns the time of the reply in milliseconds.
func pingOnce(ip string) (float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping")
	pingCommand(cmd, ip)
	out, _ := cmd.CombinedOutput() // ping fails when the packet is lost; the output tells
	for _, l := range strings.Split(string(out), "\n") {
		if !strings.Contains(strings.ToLower(l), "ttl") {
			continue // only reply lines have a TTL
		}
		if m := pingTime.FindStringSubmatch(l); m != nil {
			if v, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// ---- DNS lookup ----

// dnsLookup shows the main DNS records of a name, or the name of an IP address.
func dnsLookup(input string) ([]Result, error) {
	host := cleanHost(input)
	if host == "" {
		return nil, errors.New("please type a domain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := net.DefaultResolver

	if net.ParseIP(host) != nil {
		names, err := r.LookupAddr(ctx, host)
		if err != nil || len(names) == 0 {
			return nil, fmt.Errorf("no name found for %s", host)
		}
		var results []Result
		for _, n := range names {
			results = append(results, Result{Name: "Name (PTR)", Value: strings.TrimSuffix(n, ".")})
		}
		return results, nil
	}

	// The lookups are independent, so they run at the same time.
	var mu sync.Mutex
	found := map[string][]string{}
	add := func(kind string, values ...string) {
		mu.Lock()
		found[kind] = append(found[kind], values...)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	run(func() {
		if ips, err := r.LookupIP(ctx, "ip4", host); err == nil {
			for _, ip := range ips {
				add("A", ip.String())
			}
		}
	})
	run(func() {
		if ips, err := r.LookupIP(ctx, "ip6", host); err == nil {
			for _, ip := range ips {
				add("AAAA", ip.String())
			}
		}
	})
	run(func() {
		if c, err := r.LookupCNAME(ctx, host); err == nil && !strings.EqualFold(strings.TrimSuffix(c, "."), host) {
			add("CNAME", strings.TrimSuffix(c, "."))
		}
	})
	run(func() {
		if mxs, err := r.LookupMX(ctx, host); err == nil {
			for _, mx := range mxs {
				add("MX", fmt.Sprintf("%s (priority %d)", strings.TrimSuffix(mx.Host, "."), mx.Pref))
			}
		}
	})
	run(func() {
		if nss, err := r.LookupNS(ctx, host); err == nil {
			for _, ns := range nss {
				add("NS", strings.TrimSuffix(ns.Host, "."))
			}
		}
	})
	run(func() {
		if txts, err := r.LookupTXT(ctx, host); err == nil {
			add("TXT", txts...)
		}
	})
	wg.Wait()

	var results []Result
	for _, kind := range []string{"CNAME", "A", "AAAA", "MX", "NS", "TXT"} {
		for _, v := range found[kind] {
			results = append(results, Result{Name: kind, Value: v})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no records found for %s", host)
	}
	return results, nil
}

// ---- Port check ----

// commonPorts are checked when the user types only a host.
var commonPorts = []struct {
	port int
	name string
}{
	{21, "FTP"}, {22, "SSH"}, {25, "SMTP"}, {53, "DNS"}, {80, "HTTP"}, {110, "POP3"},
	{143, "IMAP"}, {443, "HTTPS"}, {445, "SMB"}, {465, "SMTPS"}, {587, "SMTP"},
	{993, "IMAPS"}, {995, "POP3S"}, {1433, "SQL Server"}, {3306, "MySQL"},
	{3389, "Remote Desktop"}, {5432, "PostgreSQL"}, {6379, "Redis"}, {8080, "HTTP alt"}, {8443, "HTTPS alt"},
}

const portTimeout = 3 * time.Second

// portCheck tells whether a port accepts connections. With only a host, it checks the common ports.
func portCheck(input string) ([]Result, error) {
	host, port := splitHostPort(input)
	if host == "" {
		return nil, errors.New("please type an address, e.g. example.com:22")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ips, err := resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	ip := ips[0].String()

	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("%q is not a valid port (1-65535)", port)
		}
		label := fmt.Sprintf("Port %d", n)
		for _, p := range commonPorts {
			if p.port == n {
				label += " (" + p.name + ")"
			}
		}
		return []Result{{Name: label, Value: dialPort(ip, n)}, {Name: "IP address", Value: ip}}, nil
	}

	values := make([]string, len(commonPorts))
	var wg sync.WaitGroup
	for i, p := range commonPorts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			values[i] = dialPort(ip, p.port)
		}()
	}
	wg.Wait()
	var open, closed []Result
	for i, p := range commonPorts {
		r := Result{Name: fmt.Sprintf("%d %s", p.port, p.name), Value: values[i]}
		if strings.HasPrefix(values[i], "open") {
			open = append(open, r)
		} else {
			closed = append(closed, r)
		}
	}
	return append(append(open, closed...), Result{Name: "IP address", Value: ip}), nil
}

// splitHostPort accepts "host:port", "host port", "[::1]:22" or just a host.
func splitHostPort(input string) (string, string) {
	s := strings.TrimSpace(input)
	if f := strings.Fields(s); len(f) == 2 {
		return cleanHost(f[0]), f[1]
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			return u.Hostname(), u.Port()
		}
	}
	if h, p, err := net.SplitHostPort(s); err == nil {
		return cleanHost(h), p
	}
	return cleanHost(s), ""
}

func dialPort(ip string, port int) string {
	start := time.Now()
	c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), portTimeout)
	switch {
	case err == nil:
		c.Close()
		return fmt.Sprintf("open · %d ms", time.Since(start).Milliseconds())
	case connRefused(err):
		return "closed"
	default:
		return "no answer (blocked by a firewall or the host is down)"
	}
}
