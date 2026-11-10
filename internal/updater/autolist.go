package updater

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hoaxisr/awg-manager/internal/downloader"
	"github.com/hoaxisr/awg-manager/internal/sys/appver"
)

// DownloadAutolistPackage downloads the satellite from the same release as AWGM updates.
func (s *Service) DownloadAutolistPackage(ctx context.Context, arch, destPath string) error {
	switch arch {
	case "mipsel-3.4", "mips-3.4", "aarch64-3.10":
	default:
		return fmt.Errorf("unsupported awgmautolist architecture: %q", arch)
	}
	if releaseBaseURL == "" {
		return fmt.Errorf("release URL is not configured")
	}
	version, err := fetchLatestVersionWithDownloader(ctx, s.downloader, releaseAssetURL("VERSION"), nil)
	if err != nil {
		return fmt.Errorf("release version: %w", err)
	}
	if !safeIPKFilenameRe.MatchString(version) {
		return fmt.Errorf("invalid release version: %q", version)
	}
	filename := fmt.Sprintf("awgmautolist_%s_%s-kn.ipk", version, arch)
	_, err = s.downloader.DownloadFile(ctx, downloader.FileRequest{
		Request: downloader.Request{
			Purpose:      "awgmautolist-ipk",
			UserAgent:    appver.UA(),
			URL:          releaseAssetURL(filename),
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
	return nil
}
