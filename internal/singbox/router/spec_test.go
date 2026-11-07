package router

import (
	"context"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

func TestBuildTproxySpec_DeviceBypassDNSRescue(t *testing.T) {
	for _, tc := range []struct {
		name       string
		policyMode bool
		bypass     bool
		isolated   bool
		wantRescue bool
	}{
		{"legacy policy", true, false, false, true},
		{"isolated policy", true, false, true, false},
		{"legacy all devices", false, false, false, false},
		{"isolated all devices", false, false, true, false},
		{"legacy bypass", false, true, false, true},
		{"isolated bypass", false, true, true, false},
		{"legacy policy bypass", true, true, false, true},
		{"isolated policy bypass", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sb := newTestSingbox(t)
			sb.replyIsolated = tc.isolated
			svc := newReconcileInstalledService(t, sb)
			discovered := false
			stubLANBridges(t, func(_ context.Context, mark string) ([]LANBridgeDNSRedir, error) {
				discovered = true
				if mark != "0xff" {
					t.Errorf("discovery mark = %q, want 0xff", mark)
				}
				return []LANBridgeDNSRedir{{Bridge: "br0", Port: 41100}}, nil
			})
			sr := storage.SingboxRouterSettings{}
			if tc.bypass {
				sr.BypassDevices = []string{"aa:bb:cc:dd:ee:ff"}
			}
			spec := svc.buildTproxySpec(context.Background(), sr, "0xff", tc.policyMode, nil, nil)
			if discovered != tc.wantRescue {
				t.Errorf("LAN discovery = %v, want %v", discovered, tc.wantRescue)
			}
			if got := strings.Contains(buildNatRestoreInput(spec), DNSRescueTag); got != tc.wantRescue {
				t.Errorf("DNS rescue rules = %v, want %v", got, tc.wantRescue)
			}
		})
	}
}
