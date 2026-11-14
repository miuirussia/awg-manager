package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/downloader"
	"github.com/hoaxisr/awg-manager/internal/sys/appver"
	"github.com/hoaxisr/awg-manager/internal/sys/semver"
)

// DownloadAutolistPackage downloads the satellite from the update channel.
// Develop builds use their own channel even when panel updates track stable.
func (s *Service) DownloadAutolistPackage(ctx context.Context, arch, destPath string) error {
	switch arch {
	case "mipsel-3.4", "mips-3.4", "aarch64-3.10":
	default:
		return fmt.Errorf("unsupported awgmautolist architecture: %q", arch)
	}
	channel := s.channel()
	if strings.Contains(s.version, "+r") {
		channel = channelDevelop
	}
	if channelBaseURL(channel) == "" {
		return fmt.Errorf("release URL is not configured")
	}
	pkg, err := fetchLatestPackageWithDownloader(
		ctx,
		s.downloader,
		releaseAssetURL(channel, "Packages-"+arch+".gz"),
		"awgmautolist",
		semver.CompareWithRevision,
		nil,
	)
	if err != nil {
		return fmt.Errorf("satellite package index: %w", err)
	}
	filename := fmt.Sprintf("awgmautolist_%s_%s-kn.ipk", pkg.Version, arch)
	if !safeIPKFilenameRe.MatchString(pkg.Version) || pkg.Filename != filename {
		return fmt.Errorf("invalid satellite package: %q", pkg.Filename)
	}
	_, err = s.downloader.DownloadFile(ctx, downloader.FileRequest{
		Request: downloader.Request{
			Purpose:      "awgmautolist-ipk",
			UserAgent:    appver.UA(),
			URL:          releaseAssetURL(channel, filename),
			Method:       http.MethodGet,
			Timeout:      downloadTimeout,
			MaxBodyBytes: ipkMaxBytes,
		},
		DestPath:     destPath,
		MaxFileBytes: ipkMaxBytes,
		Mode:         0o600,
		Atomic:       true,
	})
	if err != nil {
		return fmt.Errorf("download satellite IPK: %w", err)
	}
	if err := verifyFileSHA256(destPath, pkg.SHA256); err != nil {
		os.Remove(destPath)
		return err
	}
	return nil
}
