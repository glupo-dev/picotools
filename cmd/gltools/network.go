package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"time"
)

// myIP returns the local addresses of this computer, its public address and its name.
func myIP() ([]Result, error) {
	var results []Result

	// The interface used to reach the internet goes first. No data is sent by this "dial".
	primary := ""
	if c, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		primary = c.LocalAddr().(*net.UDPAddr).IP.String()
		c.Close()
	}

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			r := Result{Name: "Local (" + iface.Name + ")", Value: ipnet.IP.String()}
			if r.Value == primary {
				results = append([]Result{r}, results...)
			} else {
				results = append(results, r)
			}
		}
	}

	if ip, err := publicIP(); err == nil {
		results = append(results, Result{Name: "Public", Value: ip})
	} else {
		results = append(results, Result{Name: "Public", Value: "not available (no internet connection?)"})
	}

	if name, err := os.Hostname(); err == nil {
		results = append(results, Result{Name: "Computer name", Value: name})
	}
	return results, nil
}

// publicIP asks an external service which address our requests come from.
func publicIP() (string, error) {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", errors.New("unexpected answer")
	}
	return ip, nil
}

const websiteTimeout = 10 * time.Second

// checkWebsite tells whether a website answers, how fast, and a few details about it.
func checkWebsite(address string) ([]Result, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("please type an address")
	}
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a valid address", address)
	}

	var remoteIP string
	var resolved []net.IP
	var firstByte time.Time
	trace := &httptrace.ClientTrace{
		DNSDone: func(info httptrace.DNSDoneInfo) {
			for _, a := range info.Addrs {
				resolved = append(resolved, a.IP)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			if a, ok := info.Conn.RemoteAddr().(*net.TCPAddr); ok {
				remoteIP = a.IP.String()
			}
		},
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}

	ctx, cancel := context.WithTimeout(context.Background(), websiteTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GL Tools")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, websiteError(u.Hostname(), resolved, err)
	}
	resp.Body.Close()
	if firstByte.IsZero() {
		firstByte = time.Now()
	}

	results := []Result{
		{Name: "Status", Value: resp.Status},
		{Name: "Response time", Value: fmt.Sprintf("%d ms", firstByte.Sub(start).Milliseconds())},
	}
	if remoteIP != "" {
		results = append(results, Result{Name: "IP address", Value: remoteIP})
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry := resp.TLS.PeerCertificates[0].NotAfter
		days := int(time.Until(expiry).Hours() / 24)
		results = append(results, Result{Name: "Certificate", Value: fmt.Sprintf("valid until %s (%d days)", expiry.Format("2006-01-02"), days)})
	}
	if final := resp.Request.URL.String(); final != u.String() {
		results = append(results, Result{Name: "Final URL", Value: final})
	}
	return results, nil
}

// websiteError turns network errors into short, readable messages.
// ips are the addresses the name was resolved to, used to explain timeouts.
func websiteError(host string, ips []net.IP, err error) error {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &dnsErr):
		return fmt.Errorf("address not found: %s", host)
	case errors.Is(err, context.DeadlineExceeded):
		msg := fmt.Sprintf("no answer within %d seconds", int(websiteTimeout.Seconds()))
		if len(ips) == 0 {
			return errors.New(msg)
		}
		list := make([]string, len(ips))
		allPrivate := true
		for i, ip := range ips {
			list[i] = ip.String()
			allPrivate = allPrivate && (ip.IsPrivate() || ip.IsLoopback())
		}
		msg += " from " + strings.Join(list, ", ")
		if allPrivate {
			msg += ". These are internal addresses: on this network the name points to internal servers, not to the public website"
		}
		return errors.New(msg)
	case errors.As(err, &certErr):
		return fmt.Errorf("the security certificate of %s is not valid", host)
	case connRefused(err):
		return fmt.Errorf("%s refused the connection", host)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
