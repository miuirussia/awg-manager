package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
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
			index := fmt.Sprintf(
				"Package: awgmautolist\nVersion: 2.20.0+r3\nFilename: %s\nSHA256sum: %x\n",
				filename,
				sha256.Sum256([]byte("satellite IPK")),
			)
			compressedIndex := gzipBytes(t, index)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/Packages-" + arch + ".gz":
					w.Write(compressedIndex)
				case "/" + filename:
					w.Write([]byte("satellite IPK"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			withMockRepo(t, srv)
			s := &Service{downloader: newDefaultDownloader()}
			dest := filepath.Join(t.TempDir(), "satellite.ipk")
			if err := s.DownloadAutolistPackage(context.Background(), arch, dest); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != "satellite IPK" {
				t.Fatalf("download = %q, %v", got, err)
			}
			if got, want := strings.Join(paths, ","), "/Packages-"+arch+".gz,/"+filename; got != want {
				t.Fatalf("requests = %q, want %q", got, want)
			}
		})
	}
}

func TestDownloadAutolistPackageRejectsCorruptIPK(t *testing.T) {
	index := "Package: awgmautolist\nVersion: 2.20.0\n" +
		"Filename: awgmautolist_2.20.0_mipsel-3.4-kn.ipk\n" +
		"SHA256sum: " + strings.Repeat("a", 64) + "\n"
	s := &Service{downloader: &fakeDownloader{
		readAllFn: func(context.Context, downloader.Request) ([]byte, downloader.ResponseMeta, error) {
			return gzipBytes(t, index), downloader.ResponseMeta{}, nil
		},
		downloadFileFn: func(_ context.Context, req downloader.FileRequest) (downloader.FileResult, error) {
			return downloader.FileResult{}, os.WriteFile(req.DestPath, []byte("corrupt"), 0o600)
		},
	}}
	dest := filepath.Join(t.TempDir(), "satellite.ipk")
	if err := s.DownloadAutolistPackage(context.Background(), "mipsel-3.4", dest); err == nil {
		t.Fatal("corrupt IPK accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("corrupt IPK was not removed: %v", err)
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
					return gzipBytes(t, "Package: awgmautolist\nVersion: "+tc.version+"\nFilename: awgmautolist_"+tc.version+"_"+tc.arch+"-kn.ipk\n"), downloader.ResponseMeta{}, nil
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
