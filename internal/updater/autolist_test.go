package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/downloader"
)

func TestDownloadAutolistPackage(t *testing.T) {
	for _, arch := range []string{"mipsel-3.4", "mips-3.4", "aarch64-3.10"} {
		t.Run(arch, func(t *testing.T) {
			var paths []string
			filename := "awgmautolist_2.20.0+r3_" + arch + "-kn.ipk"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/VERSION":
					w.Write([]byte("2.20.0+r3\n"))
				case "/" + filename:
					w.Write([]byte("satellite IPK"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			withMockRelease(t, srv)
			s := &Service{downloader: newDefaultDownloader()}
			dest := filepath.Join(t.TempDir(), "satellite.ipk")
			if err := s.DownloadAutolistPackage(context.Background(), arch, dest); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != "satellite IPK" {
				t.Fatalf("download = %q, %v", got, err)
			}
			if got, want := strings.Join(paths, ","), "/VERSION,/"+filename; got != want {
				t.Fatalf("requests = %q, want %q", got, want)
			}
		})
	}
}

func TestDownloadAutolistPackageRejectsInvalidRelease(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		arch    string
	}{
		{name: "unsupported architecture", version: "2.20.0", arch: "amd64"},
		{name: "empty version", version: "\n", arch: "mipsel-3.4"},
		{name: "path traversal", version: "../../other", arch: "mipsel-3.4"},
		{name: "URL query", version: "2.20.0?x=1", arch: "mipsel-3.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := releaseBaseURL
			releaseBaseURL = "https://release.test"
			t.Cleanup(func() { releaseBaseURL = old })
			downloaded := false
			s := &Service{downloader: &fakeDownloader{
				readAllFn: func(context.Context, downloader.Request) ([]byte, downloader.ResponseMeta, error) {
					return []byte(tc.version), downloader.ResponseMeta{}, nil
				},
				downloadFileFn: func(context.Context, downloader.FileRequest) (downloader.FileResult, error) {
					downloaded = true
					return downloader.FileResult{}, nil
				},
			}}
			if err := s.DownloadAutolistPackage(context.Background(), tc.arch, filepath.Join(t.TempDir(), "satellite.ipk")); err == nil {
				t.Fatal("invalid release accepted")
			}
			if downloaded {
				t.Fatal("package requested for invalid release")
			}
		})
	}
}
