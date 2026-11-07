package router

import (
	"reflect"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

func TestNormalizeSingboxRouterSettings_BypassDevices(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		macs []string
		want []string
		bad  bool
	}{
		{name: "empty"},
		{name: "normalize and deduplicate", macs: []string{" AA-BB-CC-DD-EE-FF ", "aa:bb:cc:dd:ee:ff", "02:11:22:33:44:55"}, want: []string{"aa:bb:cc:dd:ee:ff", "02:11:22:33:44:55"}},
		{name: "invalid", macs: []string{"invalid"}, bad: true},
		{name: "injection", macs: []string{"aa:bb:cc:dd:ee:ff\nCOMMIT"}, bad: true},
		{name: "empty address", macs: []string{""}, bad: true},
		{name: "zero", macs: []string{"00:00:00:00:00:00"}, bad: true},
		{name: "multicast", macs: []string{"01:00:5e:00:00:01"}, bad: true},
		{name: "broadcast", macs: []string{"ff:ff:ff:ff:ff:ff"}, bad: true},
		{name: "eui64", macs: []string{"02:11:22:33:44:55:66:77"}, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]string(nil), tc.macs...)
			sr, err := NormalizeSingboxRouterSettings(storage.SingboxRouterSettings{WANAutoDetect: true, BypassDevices: tc.macs})
			if (err != nil) != tc.bad {
				t.Fatalf("error = %v, want bad=%v", err, tc.bad)
			}
			if !tc.bad && !reflect.DeepEqual(sr.BypassDevices, tc.want) {
				t.Errorf("devices = %v, want %v", sr.BypassDevices, tc.want)
			}
			if !reflect.DeepEqual(tc.macs, before) {
				t.Fatal("normalization mutated the caller's slice")
			}
		})
	}
}
