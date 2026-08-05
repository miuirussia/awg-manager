package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const TLSResolveInterval = 5 * time.Hour

// TLSResolveStore keeps the hostname outside of the sing-box runtime config:
// sing-box dials an IP, while the editor continues to show the hostname.
type TLSResolveStore struct {
	mu      sync.Mutex
	path    string
	Entries map[string]TLSResolveEntry `json:"entries"`
}
type TLSResolveEntry struct {
	Host       string    `json:"host"`
	LastIP     string    `json:"lastIP"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

func NewTLSResolveStore(dataDir string) *TLSResolveStore {
	return &TLSResolveStore{path: filepath.Join(dataDir, "singbox_tls_resolve.json"), Entries: map[string]TLSResolveEntry{}}
}
func (s *TLSResolveStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, s)
}
func (s *TLSResolveStore) save() error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}
func (s *TLSResolveStore) Get(tag string) (TLSResolveEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Entries[tag]
	return v, ok
}
func (s *TLSResolveStore) Put(tag string, v TLSResolveEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Entries[tag] = v
	return s.save()
}
func (s *TLSResolveStore) Delete(tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Entries, tag)
	return s.save()
}
func (s *TLSResolveStore) Rename(oldTag, newTag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.Entries[oldTag]; ok {
		delete(s.Entries, oldTag)
		s.Entries[newTag] = v
		return s.save()
	}
	return nil
}
func (r *TLSResolver) Delete(tag string) error            { return r.store.Delete(tag) }
func (r *TLSResolver) Rename(oldTag, newTag string) error { return r.store.Rename(oldTag, newTag) }
func (r *TLSResolver) DisplayHost(tag, fallback string) string {
	if e, ok := r.store.Get(tag); ok {
		return e.Host
	}
	return fallback
}

// TLSResolver updates tunnel endpoint IPs while preserving their source host.
type TLSResolver struct {
	op     *Operator
	store  *TLSResolveStore
	lookup func(context.Context, string) ([]string, error)
	mu     sync.Mutex
	busy   map[string]bool
}

func NewTLSResolver(op *Operator, store *TLSResolveStore) *TLSResolver {
	return &TLSResolver{op: op, store: store, lookup: func(ctx context.Context, host string) ([]string, error) {
		return net.DefaultResolver.LookupHost(ctx, host)
	}, busy: map[string]bool{}}
}
func tlsTunnel(ob map[string]any) bool {
	t, _ := ob["type"].(string)
	return t == "vless" || t == "trojan" || t == "hysteria2"
}
func firstIPv4(ips []string) string {
	for _, v := range ips {
		if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}
func (r *TLSResolver) Resolve(ctx context.Context, tag string, outbound json.RawMessage) (json.RawMessage, []string, error) {
	var ob map[string]any
	if err := json.Unmarshal(outbound, &ob); err != nil {
		return nil, nil, err
	}
	if !tlsTunnel(ob) {
		return outbound, nil, fmt.Errorf("tunnel does not support TLS resolution")
	}
	host, _ := ob["server"].(string)
	if old, ok := r.store.Get(tag); ok {
		host = old.Host
	}
	if net.ParseIP(host) != nil || host == "" {
		return nil, nil, fmt.Errorf("server hostname is required")
	}
	ips, err := r.lookup(ctx, host)
	if err != nil {
		return nil, nil, err
	}
	ip := firstIPv4(ips)
	if ip == "" {
		return nil, ips, fmt.Errorf("resolve %q: no IPv4 address", host)
	}
	ob["server"] = ip
	if tls, ok := ob["tls"].(map[string]any); ok {
		if s, _ := tls["server_name"].(string); s == "" {
			tls["server_name"] = host
		}
	}
	next, err := json.Marshal(ob)
	if err != nil {
		return nil, nil, err
	}
	if err = r.op.UpdateTunnel(ctx, tag, next); err != nil {
		return nil, nil, err
	}
	if err = r.store.Put(tag, TLSResolveEntry{Host: host, LastIP: ip, ResolvedAt: time.Now().UTC()}); err != nil {
		return nil, nil, err
	}
	return next, ips, nil
}
func (r *TLSResolver) Overlay(tag string, raw json.RawMessage) json.RawMessage {
	e, ok := r.store.Get(tag)
	if !ok {
		return raw
	}
	var ob map[string]any
	if json.Unmarshal(raw, &ob) != nil {
		return raw
	}
	ob["server"] = e.Host
	out, _ := json.Marshal(ob)
	return out
}
func (r *TLSResolver) RefreshDue(ctx context.Context) {
	for tag, e := range r.store.Entries {
		if time.Since(e.ResolvedAt) < TLSResolveInterval {
			continue
		}
		raw, err := r.op.GetTunnel(ctx, tag)
		if err == nil {
			_, _, _ = r.Resolve(ctx, tag, raw)
		}
	}
}
