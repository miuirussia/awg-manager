package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	osexec "os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hoaxisr/awg-manager/internal/downloader"
	"github.com/hoaxisr/awg-manager/internal/sys/appver"
	"github.com/hoaxisr/awg-manager/internal/sys/semver"
)

const (
	defaultEntwareRepoURL = "http://repo.hoaxisr.ru"
	repoTimeout           = 30 * time.Second
	downloadTimeout       = 5 * time.Minute
	downloadDir           = "/opt/tmp"
	pkgName               = "awg-manager"
)

const (
	channelStable  = "stable"
	channelDevelop = "develop"
)

// entwareRepoURL is a variable so tests can override it with httptest server URL.
var entwareRepoURL = defaultEntwareRepoURL

// releaseBaseURL is a variable so tests can override it with httptest server URL.
var releaseBaseURL = ""

// channelBaseURL возвращает базовый URL репозитория для канала. develop
// отдаётся из подкаталога /develop того же сервера.
func channelBaseURL(channel string) string {
	if channel == channelDevelop {
		return entwareRepoURL + "/develop"
	}
	return entwareRepoURL
}

// checkWithDownloader returns information about an available update.
// stats contains anonymous installation-statistics headers, or nil.
func checkWithDownloader(ctx context.Context, currentVersion, channel string, dl Downloader, stats http.Header) *UpdateInfo {
	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		CheckedAt:      time.Now(),
	}

	if channel == channelDevelop {
		return checkDevelopWithDownloader(ctx, currentVersion, dl, info, stats)
	}
	return checkStableWithDownloader(ctx, currentVersion, dl, info, stats)
}

func checkDevelopWithDownloader(ctx context.Context, currentVersion string, dl Downloader, info *UpdateInfo, stats http.Header) *UpdateInfo {
	base := channelBaseURL(channelDevelop)
	archDir := archSuffixToRepoDir(archSuffix())
	pkgsURL := fmt.Sprintf("%s/%s/Packages.gz", base, archDir)

	pkg, err := fetchLatestPackageWithDownloader(ctx, dl, pkgsURL, pkgName, semver.CompareWithRevision, stats)
	if err != nil {
		info.Error = fmt.Sprintf("entware repo: %s", err)
		return info
	}

	if semver.CompareWithRevision(currentVersion, pkg.Version) >= 0 {
		return info
	}

	info.Available = true
	info.LatestVersion = pkg.Version
	info.DownloadURL = fmt.Sprintf("%s/%s/%s", base, archDir, pkg.Filename)
	info.SHA256 = pkg.SHA256
	return info
}

// upgradeLogPath captures the detached opkg output. The old daemon is
// stopped by the package prerm mid-install, so this file is the ONLY
// diagnostic left if opkg fails and the router ends up without awg-manager.
// /tmp — системный tmpfs (RAM): не пишем на накопитель, не переживает ребут.
// Переменная — тесты подменяют путь.
var upgradeLogPath = "/tmp/awg-manager-upgrade.log"

// installedBinPath — куда пакет кладёт бинарь; проверяется после opkg install.
const installedBinPath = "/opt/bin/awg-manager"

// buildUpgradeScript собирает detached-скрипт установки: opkg + верификация
// бинаря (не пустой + ELF-магия) + исход в syslog роутера через logger —
// единственный канал, который виден пользователю и без демона, и без нашего
// веба (#571: сбойная запись на накопитель оставила 0-байтный бинарь, а вся
// обвязка молча отчиталась об успехе). IPK удаляется только после успешной
// проверки — иначе остаётся для ручного opkg install.
//
// opkg update перед установкой: зависимости пакета opkg качает по локальному
// списку, а у старого списка файл зависимости на сервере уже заменён новой
// версией (F933). Код update не проверяем — без сети install всё равно
// попробуем: зависимости могут уже стоять. Провал — ненулевой код выхода и
// строка «upgrade FAILED» в логе: демон, если ещё жив, покажет причину в UI.
func buildUpgradeScript(ipkPath, binPath string) string {
	return fmt.Sprintf(`sleep 2; opkg update; opkg install %[1]s; rc=$?
if [ "$rc" = "0" ] && [ -s %[2]s ] && dd if=%[2]s bs=4 count=1 2>/dev/null | grep -q ELF; then
	rm -f %[1]s
	logger -t awg-manager "upgrade: opkg install OK, binary verified"
else
	echo "upgrade FAILED: opkg rc=$rc"
	logger -t awg-manager -p daemon.err "upgrade FAILED: opkg rc=$rc or corrupt binary %[2]s; ipk kept: %[1]s"
	exit 1
fi`, ipkPath, binPath)
}

func checkStableWithDownloader(ctx context.Context, currentVersion string, dl Downloader, info *UpdateInfo, stats http.Header) *UpdateInfo {
	latestVersion, err := fetchLatestVersionWithDownloader(ctx, dl, releaseAssetURL("VERSION"), stats)
	if err != nil {
		info.Error = fmt.Sprintf("release version: %s", err)
		return info
	}

	if compareUpdateVersions(currentVersion, latestVersion) >= 0 {
		return info
	}

	info.Available = true
	info.LatestVersion = latestVersion
	info.DownloadURL = releaseAssetURL(fmt.Sprintf("awg-manager_%s_%s-kn.ipk", latestVersion, archSuffix()))
	return info
}

// fetchLatestVersion downloads the release VERSION asset and returns its
// trimmed contents.
func fetchLatestVersionWithDownloader(ctx context.Context, dl Downloader, versionURL string, stats http.Header) (string, error) {
	if dl == nil {
		dl = newDefaultDownloader()
	}
	body, _, err := dl.ReadAll(ctx, downloader.Request{
		Purpose:      "awgm-update-check",
		UserAgent:    appver.UA(),
		Headers:      stats,
		URL:          versionURL,
		Method:       http.MethodGet,
		Timeout:      repoTimeout,
		MaxBodyBytes: releaseVersionMaxBytes,
	})
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(string(body))
	if version == "" {
		return "", fmt.Errorf("empty VERSION")
	}
	return version, nil
}

func releaseAssetURL(filename string) string {
	return strings.TrimRight(releaseBaseURL, "/") + "/" + filename
}

func compareUpdateVersions(a, b string) int {
	baseA, revA, hasRevA := splitBuildRevision(a)
	baseB, revB, hasRevB := splitBuildRevision(b)
	if cmp := semver.Compare(baseA, baseB); cmp != 0 {
		return cmp
	}
	if hasRevA && hasRevB {
		if revA < revB {
			return -1
		}
		if revA > revB {
			return 1
		}
		return 0
	}
	if hasRevA {
		return 1
	}
	if hasRevB {
		return -1
	}
	return 0
}

func splitBuildRevision(version string) (base string, rev int, ok bool) {
	base, suffix, found := strings.Cut(version, "+r")
	if !found || suffix == "" {
		return version, 0, false
	}
	rev, err := strconv.Atoi(suffix)
	if err != nil {
		return version, 0, false
	}
	return base, rev, true
}

// startDetachedUpgrade запускает установку отдельным сеансом. done (может
// быть nil) зовётся с итогом скрипта, если демон его дождался: при успехе
// prerm пакета останавливает демон раньше, так что done(nil) почти не бывает.
var startDetachedUpgrade = func(ipkPath string, done func(error)) error {
	cmd := osexec.Command("sh", "-c", buildUpgradeScript(ipkPath, installedBinPath))
	if logf, err := os.OpenFile(upgradeLogPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err == nil {
		cmd.Stdout = logf
		cmd.Stderr = logf
		defer logf.Close() // child keeps its own fd after Start
	}
	setUpgradeDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		if done != nil {
			done(err)
		}
	}()
	return nil
}

func downloadPackage(ctx context.Context, downloadURL, wantSHA256 string, dl Downloader) (string, error) {
	if dl == nil {
		dl = newDefaultDownloader()
	}
	filename, err := ipkFilenameFromURL(downloadURL)
	if err != nil {
		return "", err
	}
	ipkPath := downloadDir + "/" + filename

	_, err = dl.DownloadFile(ctx, downloader.FileRequest{
		Request: downloader.Request{
			Purpose:      "awgm-update-ipk",
			UserAgent:    appver.UA(),
			URL:          downloadURL,
			Method:       http.MethodGet,
			Timeout:      downloadTimeout,
			MaxBodyBytes: ipkMaxBytes,
		},
		DestPath:     ipkPath,
		MaxFileBytes: ipkMaxBytes,
		Mode:         0o644,
		Atomic:       true,
	})
	if err != nil {
		return "", fmt.Errorf("download IPK: %w", err)
	}
	if err := verifyFileSHA256(ipkPath, wantSHA256); err != nil {
		os.Remove(ipkPath)
		return "", err
	}
	return ipkPath, nil
}

// beforeInstall (может быть nil) зовётся с путём пакета после скачивания и
// проверки, перед установкой: снимок не пишется, если обновление не дошло до
// установки.
func upgradeWithDownloader(ctx context.Context, downloadURL, wantSHA256 string, dl Downloader, beforeInstall func(ipkPath string), done func(error)) error {
	ipkPath, err := downloadPackage(ctx, downloadURL, wantSHA256, dl)
	if err != nil {
		return err
	}
	if beforeInstall != nil {
		beforeInstall(ipkPath)
	}
	if err := startDetachedUpgrade(ipkPath, done); err != nil {
		os.Remove(ipkPath)
		return err
	}
	return nil
}

// verifyFileSHA256 checks path against the hex digest want. Empty want is
// accepted (older repo indexes without SHA256sum) — but when the index does
// provide a digest, a mismatch aborts the root opkg install of the file.
func verifyFileSHA256(path, want string) error {
	if want == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("verify IPK: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("verify IPK: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("IPK checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

func ipkFilenameFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid download URL: %w", err)
	}
	if strings.Contains(u.Path, "..") || strings.Contains(u.EscapedPath(), "..") || strings.Contains(strings.ToLower(u.EscapedPath()), "%2e") {
		return "", fmt.Errorf("invalid download URL path: %q", raw)
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("invalid download URL path: %q", raw)
	}
	if !isSafeIPKFilename(name) {
		return "", fmt.Errorf("invalid package filename %q", name)
	}
	return name, nil
}

var safeIPKFilenameRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

func isSafeIPKFilename(name string) bool {
	if name == "" || name == "." || name == "/" {
		return false
	}
	if !strings.HasPrefix(name, pkgName+"_") && !strings.HasPrefix(name, "awgmautolist_") {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(name), ".ipk") {
		return false
	}
	return safeIPKFilenameRe.MatchString(name)
}

// upgradeFailureReason вынимает из лога установки строки ошибок: «* …» из
// «Collected errors» opkg, «wget: …» и итог скрипта «upgrade FAILED: …».
// Повторы схлопываются, берутся последние maxReasonLines.
func upgradeFailureReason(log string) string {
	const maxReasonLines = 6
	var lines []string
	seen := make(map[string]bool)
	for _, l := range strings.Split(log, "\n") {
		l = strings.TrimSpace(l)
		if rest, ok := strings.CutPrefix(l, "* "); ok {
			l = rest
		} else if !strings.HasPrefix(l, "wget: ") && !strings.HasPrefix(l, "upgrade FAILED") {
			continue
		}
		if !seen[l] {
			seen[l] = true
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return "opkg install failed, see " + upgradeLogPath
	}
	if len(lines) > maxReasonLines {
		lines = lines[len(lines)-maxReasonLines:]
	}
	return strings.Join(lines, "\n")
}

// upgradeFailureHint узнаёт частые причины: DNS (busybox wget «bad address»,
// GNU wget «unable to resolve»), нет места, opkg занят другой установкой.
func upgradeFailureHint(reason string) string {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "bad address"), strings.Contains(r, "unable to resolve"):
		return "dns"
	case strings.Contains(r, "no space left"):
		return "space"
	case strings.Contains(r, "could not lock"):
		return "lock"
	}
	return ""
}
