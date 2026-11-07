package router

import (
	"context"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

func TestDeviceBypass_PrecedesInterceptionAndBlackhole(t *testing.T) {
	t.Parallel()
	const mac = "aa:bb:cc:dd:ee:ff"
	for _, all := range []bool{false, true} {
		name := "policy"
		if all {
			name = "all devices"
		}
		t.Run(name, func(t *testing.T) {
			v4 := RestoreInputSpec{MatchAll: all, PolicyMark: "0xff", BypassDevices: []string{mac}, QoSClasses: []QoSClassSpec{{DSCP: 46, TProxyPort: 52000, RedirectPort: 52001}}}
			v6 := testV6Spec()
			v6.MatchAll, v6.BypassDevices = all, []string{mac}
			for _, tc := range []struct {
				name, blob, chain, terminal string
			}{
				{"v4 UDP and DNS", buildMangleRestoreInput(v4), ChainName, "-j TPROXY"},
				{"v4 TCP", buildNatRestoreInput(v4), RedirectChain, "-j REDIRECT"},
				{"v4 engine down", buildBlackholeRestoreInput(v4), BlackholeChain, "-A " + BlackholeChain + " -j DROP"},
				{"v6 UDP and DNS", buildMangleRestoreInput6(v6), Chain6Name, "-j TPROXY"},
				{"v6 TCP", buildNatRestoreInput6(v6), Redirect6Chain, "-j REDIRECT"},
				{"v6 engine down", buildBlackholeRestoreInput6(v6), Blackhole6Chain, "-A " + Blackhole6Chain + " -j DROP"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rule := "-A " + tc.chain + " -m mac --mac-source " + mac + " -j RETURN"
					bypass, intercept := strings.Index(tc.blob, rule), strings.Index(tc.blob, tc.terminal)
					if bypass < 0 || intercept < 0 || bypass >= intercept {
						t.Fatalf("device must bypass capture/DROP first:\n%s", tc.blob)
					}
					if strings.Count(tc.blob, "--mac-source") != 1 {
						t.Fatalf("unexpected device rules:\n%s", tc.blob)
					}
				})
			}
		})
	}
}

func TestDeviceBypass_SpecsAndDrift(t *testing.T) {
	macs := []string{"aa:bb:cc:dd:ee:ff"}
	sr := storage.SingboxRouterSettings{BypassDevices: macs}
	s := &ServiceImpl{}
	v4 := s.buildTproxySpec(context.Background(), sr, "", false, nil, nil)
	v6, _, err := buildV6Spec(sr, "", false, v6Topology{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buildRestoreInput(v4), "--mac-source "+macs[0]) || !strings.Contains(buildRestoreInput6(v6), "--mac-source "+macs[0]) {
		t.Fatal("device exclusions lost from desired specs")
	}
	without4, without6 := v4, v6
	without4.BypassDevices, without6.BypassDevices = nil, nil
	if equalInstalledSpec(&v4, &without4) || equalV6Spec(&v6, &without6) || equalBlackholeSpec(&v4, &without4) || equalBlackholeSpec6(&v6, &without6) {
		t.Fatal("changing exclusions must reinstall interception rules")
	}
	if strings.Contains(buildRestoreInput(s.buildPolicyTunSpec(sr, nil, nil)), "--mac-source") {
		t.Fatal("TProxy device bypass must not be applied partially to policy-tun")
	}
}

func TestDeviceBypass_AllDevicesDNSRescue(t *testing.T) {
	t.Parallel()
	spec := RestoreInputSpec{
		MatchAll:      true,
		BypassDevices: []string{"aa:bb:cc:dd:ee:ff"},
		LANBridges:    []LANBridgeDNSRedir{{Bridge: "br0", Port: 41100}},
	}
	blob := buildNatRestoreInput(spec)
	for _, proto := range []string{"udp", "tcp"} {
		rule := "-I PREROUTING 1 -i br0 -m mac --mac-source aa:bb:cc:dd:ee:ff -m mark --mark 0x0 -m pkttype --pkt-type unicast -p " + proto + " --dport 53"
		if !strings.Contains(blob, rule+" -m comment --comment \""+DNSRescueTag+"\" -j REDIRECT --to-ports 41100") {
			t.Fatalf("excluded device DNS must reach ndnproxy instead of sing-box's :53 listener:\n%s", blob)
		}
	}
	// Other default-policy devices still need the normal sing-box capture.
	if strings.Contains(blob, "-i br0 -m mark --mark 0x0") {
		t.Fatal("all-devices rescue must be scoped to the excluded MACs")
	}
	spec.BypassDevices = nil
	if strings.Contains(buildNatRestoreInput(spec), DNSRescueTag) {
		t.Fatal("all-devices mode without exclusions must not install DNS rescue")
	}
}
