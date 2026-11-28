package api

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/singbox/router"
)

func TestDesyncProbeOptions(t *testing.T) {
	got, err := desyncProbeOptions(router.Outbound{
		Type: "desync", Tag: "dpi", Strategy: "mode=split",
		DomainResolver: &router.DomainResolver{Server: "dns-direct"},
		DesyncFields:   router.DesyncFields{Preset: "auto", TTL: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"preset":"auto","strategy":"mode=split","ttl":4}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := desyncProbeOptions(router.Outbound{Type: "selector", Tag: "g"}); err == nil {
		t.Error("non-desync outbound must be rejected")
	}
}

func TestDesyncProbeOptionsPreservesZeroMaxBuffer(t *testing.T) {
	input := []byte(`{"type":"desync","tag":"dpi","preset":"auto","adaptive":{"max_buffer":0}}`)
	var dto SingboxRouterOutboundDTO
	if err := json.Unmarshal(input, &dto); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var outbound router.Outbound
	if err := json.Unmarshal(raw, &outbound); err != nil {
		t.Fatal(err)
	}
	got, err := desyncProbeOptions(outbound)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"adaptive":{"max_buffer":0},"preset":"auto"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestProbeErrorText(t *testing.T) {
	stderr := "Error: ttl and udp_ttl must be 1..255\nUsage:\n  awgm-box desync probe [flags]\nFATAL ttl\n"
	if got := probeErrorText(stderr, errors.New("exit status 1")); got != "ttl and udp_ttl must be 1..255" {
		t.Errorf("got %q", got)
	}
	if got := probeErrorText("", errors.New("exec: not found")); got != "exec: not found" {
		t.Errorf("got %q", got)
	}
}

// The same Outbound JSON is used for persisted slots and desync check/probe.
func TestDesyncProbeOptionsExtendedStrategy(t *testing.T) {
	input := []byte(`{"type":"desync","tag":"dpi","domain_resolver":{"server":"dns-direct"},
		"preset":"auto","mode":"hostfakesplit","split_pos":"sni+1,endhost-1",
		"tlsrec_pos":"midsld","segment_order":"2,1,3","tlsrec_first":true,
		"fake_placement":"before_between","fake_delay":"0s","fake_tls_random":false,
		"fake_tls_size":0,"fake_tls_template":"compact","http_mod":"hcsmix,rmspace",
		"hostfake_mode":"legacy","autottl":false,"udp_mode":"auto",
		"quic_desync":true,"quic_repeats":0,"quic_split_pos":"sni+1,endhost-1","quic_order":"reverse",
		"strategy":"fake_tls_random=true tlsrec_first=false",
		"adaptive":{"max_buffer":32768,"network_id":"wan-1"}}`)
	var outbound router.Outbound
	if err := json.Unmarshal(input, &outbound); err != nil {
		t.Fatal(err)
	}
	got, err := desyncProbeOptions(outbound)
	if err != nil {
		t.Fatal(err)
	}
	var wantFields, gotFields map[string]any
	if err := json.Unmarshal(input, &wantFields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"type", "tag", "domain_resolver"} {
		delete(wantFields, key)
	}
	if err := json.Unmarshal(got, &gotFields); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Errorf("strategy fields lost or changed:\n got %s\nwant %v", got, wantFields)
	}
}
