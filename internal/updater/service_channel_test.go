package updater

import "testing"

func TestChangelogURLForChannel(t *testing.T) {
	cases := []struct {
		channel string
		want    string
	}{
		{"stable", defaultChangelogURL},
		{"develop", "http://repo.hoaxisr.ru/develop/CHANGELOG.md"},
		{"", defaultChangelogURL},
	}
	for _, c := range cases {
		if got := changelogURLForChannel(c.channel); got != c.want {
			t.Errorf("changelogURLForChannel(%q) = %q, want %q", c.channel, got, c.want)
		}
	}
}

func TestChangelogURLForChannel_UsesBuildURLForStableAndEntwareForDevelop(t *testing.T) {
	oldRepo := entwareRepoURL
	oldChangelog := changelogURL
	entwareRepoURL = "http://example.test"
	changelogURL = "http://release.test/CHANGELOG.md"
	t.Cleanup(func() {
		entwareRepoURL = oldRepo
		changelogURL = oldChangelog
	})

	if got := changelogURLForChannel("stable"); got != "http://release.test/CHANGELOG.md" {
		t.Errorf("stable = %q, want http://release.test/CHANGELOG.md", got)
	}
	if got := changelogURLForChannel("develop"); got != "http://example.test/develop/CHANGELOG.md" {
		t.Errorf("develop = %q, want http://example.test/develop/CHANGELOG.md", got)
	}
}
