package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/autolistdetect"
	"github.com/hoaxisr/awg-manager/internal/logging"
	"github.com/hoaxisr/awg-manager/internal/testutil"
)

// autolistLogRecorder собирает записи, отправленные в AppLogger, чтобы
// тесты видели, какие строки журнала сателлита ушли в общий журнал.
type autolistLogRecorder struct {
	mu        sync.Mutex
	actions   []string
	msgs      []string
	levels    []string
	subgroups []string
}

func (r *autolistLogRecorder) AppLog(level logging.Level, _, subgroup, action, _, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, action)
	r.msgs = append(r.msgs, message)
	r.levels = append(r.levels, string(level))
	r.subgroups = append(r.subgroups, subgroup)
}

func (r *autolistLogRecorder) lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.msgs))
	copy(out, r.msgs)
	return out
}

func newAutolistLogHandler(t *testing.T, path string) (*AutolistHandler, *autolistLogRecorder) {
	t.Helper()
	rec := &autolistLogRecorder{}
	h := NewAutolistHandler(autolistdetect.NewService())
	h.SetLogger(rec)
	h.logSync = &autolistLogSync{path: path, configPath: filepath.Join(filepath.Dir(path), "config.json")}
	return h, rec
}

func writeAutolistLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
}

func appendAutolistLog(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("append log: %v", err)
	}
}

const testAutolistLogPrefix = "awgmautolist 2026/11/28 12:00:00.000000 "

func markedAutolistLog(level, message string) string {
	return testAutolistLogPrefix + level + " " + message
}

func journalAutolistMessage(message string) string {
	return testAutolistLogPrefix + message
}

// syncAutolistLogViaStatus тянет журнал через сам эндпоинт /status —
// проверяет и то, что sync подключён к Status, и что файл его не валит.
func syncAutolistLogViaStatus(t *testing.T, h *AutolistHandler) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/autolist/status", nil)
	rr := httptest.NewRecorder()
	h.Status(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAutolistLog_FirstImportBounded: файл больше 64 KiB — в общий журнал
// уходит не глубже последних 64 KiB и не больше 200 строк (хвост файла).
func TestAutolistLog_FirstImportBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	line := func(i int) string { return fmt.Sprintf("line-%04d %s", i, strings.Repeat("x", 91)) } // 102 байта с \n
	var sb strings.Builder
	for i := 0; i < 700; i++ {
		sb.WriteString(markedAutolistLog("INFO", line(i)) + "\n")
	}
	writeAutolistLog(t, path, sb.String()) // больше 64 KiB

	h, rec := newAutolistLogHandler(t, path)
	syncAutolistLogViaStatus(t, h)

	lines := rec.lines()
	if len(lines) != autolistLogImportLines {
		t.Fatalf("первый импорт: хотим %d строк, получили %d", autolistLogImportLines, len(lines))
	}
	if lines[0] != journalAutolistMessage(line(500)) || lines[len(lines)-1] != journalAutolistMessage(line(699)) {
		t.Errorf("импорт должен взять последние %d строк: первое=%q последнее=%q",
			autolistLogImportLines, lines[0], lines[len(lines)-1])
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, testAutolistLogPrefix) {
			t.Errorf("в импорте мусор от обрезанной головы строки: %q", l)
		}
	}
	for _, a := range rec.actions {
		if a != "autolist" {
			t.Errorf("action источника должен быть %q, получили %q", "autolist", a)
		}
	}
}

// TestAutolistLog_SecondSyncNoDuplicates: повторный опрос без новых байт
// файла не дублирует уже импортированные строки.
func TestAutolistLog_SecondSyncNoDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	writeAutolistLog(t, path, markedAutolistLog("INFO", "alpha")+"\n"+markedAutolistLog("INFO", "beta")+"\n")

	h, rec := newAutolistLogHandler(t, path)
	h.syncAutolistLog()
	h.syncAutolistLog()
	syncAutolistLogViaStatus(t, h)

	lines := rec.lines()
	if len(lines) != 2 || lines[0] != journalAutolistMessage("alpha") || lines[1] != journalAutolistMessage("beta") {
		t.Fatalf("ожидали [alpha beta] ровно один раз, получили %v", lines)
	}
}

// TestAutolistLog_AppendedLineArrives: строка, дописанная между опросами,
// доезжает целиком и один раз — в том числе если писалась по кускам.
func TestAutolistLog_AppendedLineArrives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	writeAutolistLog(t, path, markedAutolistLog("INFO", "one")+"\n")

	h, rec := newAutolistLogHandler(t, path)
	h.syncAutolistLog()
	appendAutolistLog(t, path, markedAutolistLog("INFO", "two")+"\n")
	h.syncAutolistLog()
	if lines := rec.lines(); len(lines) != 2 || lines[1] != journalAutolistMessage("two") {
		t.Fatalf("дописанная строка не доехла: %v", lines)
	}

	appendAutolistLog(t, path, markedAutolistLog("INFO", "thr"))
	h.syncAutolistLog() // неполный хвост — события нет
	if lines := rec.lines(); len(lines) != 2 {
		t.Fatalf("неполная строка не должна уходить в журнал: %v", lines)
	}
	appendAutolistLog(t, path, "ee\n")
	h.syncAutolistLog()
	lines := rec.lines()
	if len(lines) != 3 || lines[2] != journalAutolistMessage("three") {
		t.Fatalf("строка, доехавшая по кускам, должна собраться целиком: %v", lines)
	}
}

// TestAutolistLog_TruncationHandled: файл усёкся (RAMLog обнуляет при
// 5 MiB) — читаем актуальное содержимое с нуля без дублей старых строк.
func TestAutolistLog_TruncationHandled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	var old strings.Builder
	for i := 1; i <= 10; i++ {
		old.WriteString(markedAutolistLog("INFO", fmt.Sprintf("old-%d", i)) + "\n")
	}
	writeAutolistLog(t, path, old.String()) // 60 байт — offset после первого sync

	h, rec := newAutolistLogHandler(t, path)
	h.syncAutolistLog()

	// Ротация как у RAMLog: файл усёкся, новый размер меньше прочитанного
	// offset — всё содержимое новое.
	writeAutolistLog(t, path, "awgmautolist: log truncated\n"+markedAutolistLog("INFO", "fresh")+"\n")
	h.syncAutolistLog()

	want := make([]string, 0, 12)
	for i := 1; i <= 10; i++ {
		want = append(want, journalAutolistMessage(fmt.Sprintf("old-%d", i)))
	}
	want = append(want, journalAutolistMessage("fresh"))
	if lines := rec.lines(); !slices.Equal(lines, want) {
		t.Fatalf("после truncation: %v", lines)
	}
}

// TestAutolistLog_MissingFileSafe: файла нет — /status живёт, событий нет;
// когда файл появился, первый импорт отрабатывает.
func TestAutolistLog_MissingFileSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	h, rec := newAutolistLogHandler(t, path)

	syncAutolistLogViaStatus(t, h)
	syncAutolistLogViaStatus(t, h)
	if lines := rec.lines(); len(lines) != 0 {
		t.Fatalf("без файла событий быть не должно: %v", lines)
	}

	writeAutolistLog(t, path, markedAutolistLog("INFO", "late")+"\n")
	syncAutolistLogViaStatus(t, h)
	if lines := rec.lines(); len(lines) != 1 || lines[0] != journalAutolistMessage("late") {
		t.Fatalf("после появления файла должен сработать первый импорт: %v", lines)
	}
}

func TestAutolistLog_RequiresLevelAndGatesDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	content := markedAutolistLog("INFO", "info") + "\n" +
		markedAutolistLog("DEBUG", "debug-off") + "\n" +
		"legacy without level\n"
	writeAutolistLog(t, path, content)

	h, rec := newAutolistLogHandler(t, path)
	h.syncAutolistLog()
	lines := rec.lines()
	if len(lines) != 1 || lines[0] != journalAutolistMessage("info") {
		t.Fatalf("want only marked INFO, got %v", lines)
	}
	if rec.levels[0] != string(logging.LevelInfo) || rec.subgroups[0] != logging.SubAutolist {
		t.Fatalf("level=%q subgroup=%q", rec.levels[0], rec.subgroups[0])
	}

	if err := os.WriteFile(h.logSync.configPath, []byte(`{"journalDebug":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	appendAutolistLog(t, path, markedAutolistLog("DEBUG", "debug-on")+"\n")
	h.syncAutolistLog()
	lines = rec.lines()
	if len(lines) != 2 || lines[1] != journalAutolistMessage("debug-on") {
		t.Fatalf("new DEBUG must arrive without backfill: %v", lines)
	}
	if rec.levels[1] != string(logging.LevelDebug) {
		t.Fatalf("level=%q, want DEBUG", rec.levels[1])
	}
}

func TestAutolistLogWatcherDeliversAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awgmautolist.log")
	writeAutolistLog(t, path, markedAutolistLog("INFO", "initial")+"\n")
	h, rec := newAutolistLogHandler(t, path)
	h.StartLogWatcher()
	defer h.StopLogWatcher()
	if h.logWatch == nil {
		t.Fatal("watcher did not start")
	}
	if lines := rec.lines(); len(lines) != 1 {
		t.Fatalf("initial sync: %v", lines)
	}

	appendAutolistLog(t, path, markedAutolistLog("INFO", "live")+"\n")
	testutil.WaitFor(t, func() bool { return len(rec.lines()) == 2 }, "watcher did not deliver appended line")
	lines := rec.lines()
	if len(lines) != 2 || lines[1] != journalAutolistMessage("live") {
		t.Fatalf("append was not delivered by watcher: %v", lines)
	}

	h.StopLogWatcher() // Stop ждёт завершения watcher, поэтому пауза не нужна.
	appendAutolistLog(t, path, markedAutolistLog("INFO", "after-stop")+"\n")
	if lines := rec.lines(); len(lines) != 2 {
		t.Fatalf("watcher delivered after Stop: %v", lines)
	}
}
