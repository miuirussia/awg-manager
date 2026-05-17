package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/downloader"
)

// --- archSuffix sanity check (the function lives in repo.go now) ---

func TestArchSuffix(t *testing.T) {
	got := archSuffix()
	switch runtime.GOARCH {
	case "mipsle":
		if got != "mipsel-3.4" {
			t.Errorf("archSuffix() = %q, want mipsel-3.4", got)
		}
	case "mips":
		if got != "mips-3.4" {
			t.Errorf("archSuffix() = %q, want mips-3.4", got)
		}
	case "arm64":
		if got != "aarch64-3.10" {
			t.Errorf("archSuffix() = %q, want aarch64-3.10", got)
		}
	default:
		if got != runtime.GOARCH {
			t.Errorf("archSuffix() = %q, want %q (fallback)", got, runtime.GOARCH)
		}
	}
}

// --- Check with mock HTTP server returning release VERSION ---

func newMockReleaseServer(version string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/VERSION" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			w.Write([]byte(version))
		}
	}))
}

// withMockRelease points releaseBaseURL at srv.URL for the duration of the test.
func withMockRelease(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := releaseBaseURL
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = old })
}

func TestCheck_UpdateAvailable(t *testing.T) {
	arch := archSuffix()
	ipkName := "awg-manager_9.9.9+r5_" + arch + "-kn.ipk"

	srv := newMockReleaseServer("9.9.9+r5\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.0.0")

	if !info.Available {
		t.Fatal("expected Available=true")
	}
	if info.LatestVersion != "9.9.9+r5" {
		t.Errorf("LatestVersion = %q, want 9.9.9+r5", info.LatestVersion)
	}
	wantURL := srv.URL + "/" + ipkName
	if info.DownloadURL != wantURL {
		t.Errorf("DownloadURL = %q, want %q", info.DownloadURL, wantURL)
	}
	if info.Error != "" {
		t.Errorf("unexpected error: %s", info.Error)
	}
}

func TestCheck_UpdateAvailableForNewerBuildRevision(t *testing.T) {
	srv := newMockReleaseServer("2.7.10+r42\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.7.10+r41")
	if !info.Available {
		t.Fatal("expected Available=true")
	}
	if info.LatestVersion != "2.7.10+r42" {
		t.Errorf("LatestVersion = %q, want 2.7.10+r42", info.LatestVersion)
	}
}

func TestCheck_AlreadyUpToDate(t *testing.T) {
	srv := newMockReleaseServer("2.3.11+r7\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.3.11+r7")
	if info.Available {
		t.Fatal("expected Available=false (same version)")
	}
	if info.Error != "" {
		t.Errorf("unexpected error: %s", info.Error)
	}
}

func TestCheck_BuildRevisionSameAsRelease(t *testing.T) {
	srv := newMockReleaseServer("2.11.2\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.11.2+r70")
	if info.Available {
		t.Fatal("expected Available=false when release matches base of build revision")
	}
	if info.LatestVersion != "" {
		t.Errorf("LatestVersion = %q, want empty", info.LatestVersion)
	}
}

func TestCheck_NewerThanRelease(t *testing.T) {
	srv := newMockReleaseServer("2.3.10+r1\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.3.11+r1")
	if info.Available {
		t.Fatal("expected Available=false (current is newer)")
	}
}

func TestCheck_EmptyVersionFile(t *testing.T) {
	srv := newMockReleaseServer("\n", http.StatusOK)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.0.0")
	if info.Available {
		t.Fatal("expected Available=false when VERSION is empty")
	}
	if info.Error == "" {
		t.Fatal("expected error mentioning empty VERSION")
	}
}

func TestCheck_HTTPError(t *testing.T) {
	srv := newMockReleaseServer("internal error", http.StatusInternalServerError)
	defer srv.Close()
	withMockRelease(t, srv)

	info := Check(context.Background(), "2.0.0")
	if info.Available {
		t.Fatal("expected Available=false on HTTP 500")
	}
	if info.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestCheck_DevelopDetectsNewerRevision(t *testing.T) {
	arch := archSuffix()
	archDir := archSuffixToRepoDir(arch)
	ipk := "awg-manager_2.11.2+r71_" + arch + "-kn.ipk"
	packages := "Package: awg-manager\nVersion: 2.11.2+r71\nFilename: " + ipk + "\n"

	var seen downloader.Request
	dl := &fakeDownloader{
		readAllFn: func(_ context.Context, req downloader.Request) ([]byte, downloader.ResponseMeta, error) {
			seen = req
			return gzipBytes(t, packages), downloader.ResponseMeta{StatusCode: http.StatusOK}, nil
		},
	}

	info := checkWithDownloader(context.Background(), "2.11.2+r70", "develop", dl)

	if !strings.Contains(seen.URL, "/develop/") {
		t.Errorf("request URL %q does not contain /develop/", seen.URL)
	}
	wantSuffix := archDir + "/Packages.gz"
	if !strings.HasSuffix(seen.URL, wantSuffix) {
		t.Errorf("request URL %q does not end with %q", seen.URL, wantSuffix)
	}
	if !info.Available {
		t.Fatal("expected Available=true: r71 > r70 on develop")
	}
	if info.LatestVersion != "2.11.2+r71" {
		t.Errorf("LatestVersion = %q, want 2.11.2+r71", info.LatestVersion)
	}
	wantURL := entwareRepoURL + "/develop/" + archDir + "/" + ipk
	if info.DownloadURL != wantURL {
		t.Errorf("DownloadURL = %q, want %q", info.DownloadURL, wantURL)
	}
}

func TestCheck_DevelopSameRevisionUpToDate(t *testing.T) {
	arch := archSuffix()
	archDir := archSuffixToRepoDir(arch)
	ipk := "awg-manager_2.11.2+r70_" + arch + "-kn.ipk"
	packages := "Package: awg-manager\nVersion: 2.11.2+r70\nFilename: " + ipk + "\n"

	var seen downloader.Request
	dl := &fakeDownloader{
		readAllFn: func(_ context.Context, req downloader.Request) ([]byte, downloader.ResponseMeta, error) {
			seen = req
			return gzipBytes(t, packages), downloader.ResponseMeta{StatusCode: http.StatusOK}, nil
		},
	}

	info := checkWithDownloader(context.Background(), "2.11.2+r70", "develop", dl)

	if !strings.Contains(seen.URL, "/develop/") {
		t.Errorf("request URL %q does not contain /develop/", seen.URL)
	}
	wantSuffix := archDir + "/Packages.gz"
	if !strings.HasSuffix(seen.URL, wantSuffix) {
		t.Errorf("request URL %q does not end with %q", seen.URL, wantSuffix)
	}
	if info.Available {
		t.Fatal("expected Available=false: same revision")
	}
	if info.DownloadURL != "" {
		t.Errorf("DownloadURL = %q, want empty", info.DownloadURL)
	}
}
