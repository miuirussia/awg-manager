package updater

import "testing"

func TestReleaseURLsForChannel(t *testing.T) {
	oldStable, oldDevelop, oldChangelog := releaseBaseURL, developReleaseBaseURL, changelogURL
	releaseBaseURL = "https://mirror.test/stable/"
	developReleaseBaseURL = "https://mirror.test/develop/"
	changelogURL = "https://mirror.test/stable/CHANGELOG.md"
	t.Cleanup(func() { releaseBaseURL, developReleaseBaseURL, changelogURL = oldStable, oldDevelop, oldChangelog })
	for _, tc := range []struct{ name, base string }{
		{name: channelStable, base: "https://mirror.test/stable"},
		{name: channelDevelop, base: "https://mirror.test/develop"},
		{name: "", base: "https://mirror.test/stable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := channelBaseURL(tc.name); got != tc.base {
				t.Fatalf("base = %q, want %q", got, tc.base)
			}
			if got := changelogURLForChannel(tc.name); got != tc.base+"/CHANGELOG.md" {
				t.Fatalf("changelog = %q", got)
			}
		})
	}
}
