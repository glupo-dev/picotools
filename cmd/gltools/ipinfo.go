package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// cgnat is the range providers use for shared addresses (RFC 6598); like private ranges it is not public.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// ipInfo tells who an IP address belongs to and roughly where it is, using two sources:
// ipinfo.io (provider and location) and RDAP, the official registry of IP addresses
// (owner of the address block and abuse contact). With an empty input it shows our public IP.
func ipInfo(input string) ([]Result, error) {
	host := cleanHost(input)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ip := ""
	if host != "" {
		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		ip = ips[0].String()
		if p := ips[0]; p.IsPrivate() || p.IsLoopback() || p.IsLinkLocalUnicast() || cgnat.Contains(p) {
			return []Result{
				{Name: "IP address", Value: ip},
				{Name: "Type", Value: "private address of a local network: it has no public owner or location"},
			}, nil
		}
	}

	var loc ipinfoAnswer
	var reg rdapAnswer
	var locErr, regErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); locErr = getJSON(ctx, "https://ipinfo.io/"+ip+"/json", &loc) }()
	if ip != "" {
		wg.Add(1)
		go func() { defer wg.Done(); regErr = getJSON(ctx, "https://rdap.org/ip/"+ip, &reg) }()
	}
	wg.Wait()
	if ip == "" { // our own public IP: now we know it and can ask the registry
		if locErr != nil {
			return nil, fmt.Errorf("cannot find the public IP: %w", locErr)
		}
		ip = loc.IP
		regErr = getJSON(ctx, "https://rdap.org/ip/"+ip, &reg)
	}
	if locErr != nil && regErr != nil {
		return nil, fmt.Errorf("no information available (%v)", locErr)
	}

	results := []Result{{Name: "IP address", Value: ip}}
	add := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			results = append(results, Result{Name: name, Value: value})
		}
	}
	if locErr == nil {
		add("Host name", loc.Hostname)
		add("Provider", loc.Org)
		if place := joinNonEmpty(", ", loc.City, loc.Region, loc.Country); place != "" {
			add("Location", place+" (approximate)")
		}
		add("Time zone", loc.Timezone)
	} else {
		add("Location", "not available")
	}
	if regErr == nil {
		if owner := reg.owner(); !isHandle(owner) {
			add("Owner", owner)
		}
		handle := reg.Handle
		if strings.Contains(handle, " - ") {
			handle = "" // some registries use the address range as handle, already shown below
		}
		add("Network", joinNonEmpty(" · ", reg.Name, handle))
		if reg.Start != "" {
			add("Address range", reg.Start+" – "+reg.End)
		}
		add("Registry country", reg.Country)
		add("Abuse contact", reg.abuseEmail())
	} else {
		add("Owner", "registry not available")
	}
	return results, nil
}

type ipinfoAnswer struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
}

// rdapAnswer is the part of an RDAP "ip network" answer that we show.
type rdapAnswer struct {
	Handle   string       `json:"handle"`
	Name     string       `json:"name"`
	Start    string       `json:"startAddress"`
	End      string       `json:"endAddress"`
	Country  string       `json:"country"`
	Entities []rdapEntity `json:"entities"`
}

type rdapEntity struct {
	Roles    []string          `json:"roles"`
	VCard    []json.RawMessage `json:"vcardArray"`
	Entities []rdapEntity      `json:"entities"`
}

// owner is the name of the registrant, or failing that of any entity.
func (r rdapAnswer) owner() string {
	var first string
	for _, e := range r.allEntities() {
		name := e.vcard("fn")
		if e.hasRole("registrant") && name != "" {
			return name
		}
		if first == "" {
			first = name
		}
	}
	return first
}

func (r rdapAnswer) abuseEmail() string {
	for _, e := range r.allEntities() {
		if e.hasRole("abuse") {
			if mail := e.vcard("email"); mail != "" {
				return mail
			}
		}
	}
	return ""
}

// allEntities lists the entities and the ones nested inside them (abuse contacts often are).
func (r rdapAnswer) allEntities() []rdapEntity {
	var all []rdapEntity
	var walk func([]rdapEntity)
	walk = func(es []rdapEntity) {
		for _, e := range es {
			all = append(all, e)
			walk(e.Entities)
		}
	}
	walk(r.Entities)
	return all
}

func (e rdapEntity) hasRole(role string) bool {
	for _, r := range e.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// vcard returns a text property of the entity's vCard, e.g. "fn" (name) or "email".
// The vCard is ["vcard", [["fn", {}, "text", "Google LLC"], ...]].
func (e rdapEntity) vcard(prop string) string {
	if len(e.VCard) < 2 {
		return ""
	}
	var props [][]any
	if json.Unmarshal(e.VCard[1], &props) != nil {
		return ""
	}
	for _, p := range props {
		if len(p) >= 4 && p[0] == prop {
			if s, ok := p[3].(string); ok {
				return s
			}
		}
	}
	return ""
}

func getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, application/rdap+json")
	req.Header.Set("User-Agent", "GL Tools")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// isHandle tells a registry code like "INTERB-MNT" from a real name like "Google LLC".
// European registries often give only such codes; the provider line already names the company.
func isHandle(s string) bool {
	return !strings.Contains(s, " ") && strings.ToUpper(s) == s
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
