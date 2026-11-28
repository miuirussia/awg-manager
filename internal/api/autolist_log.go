package api

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/hoaxisr/awg-manager/internal/logging"
)

// autolistLogDefaultPath — журнал сателлита autolist (RAMLog в /tmp на роутере,
// см. awgmautolist/internal/logging). Панель его не пишет — только читает.
const autolistLogDefaultPath = "/tmp/awgmautolist.log"
const autolistConfigDefaultPath = "/opt/etc/awgmautolist/config.json"

const (
	// autolistLogImportBytes/autolistLogImportLines — потолки ПЕРВОГО импорта:
	// в общий журнал уходит хвост файла, а не все 5 MiB RAMLog'а сателлита.
	autolistLogImportBytes = 64 << 10
	autolistLogImportLines = 200
)

// autolistLogSync — pull-based tail журнала сателлита в app-журнал панели.
// Вызывается по файловым событиям; Status остаётся fallback. Состояние
// (offset + незавершённый хвост) живёт между вызовами под мьютексом.
// Ошибки файла проглатываются: нет файла — нет событий, /status не падает.
type autolistLogWatcher interface {
	start() error
	stop()
}

type autolistLogSync struct {
	// path/configPath — seams только для тестов; пустые — рабочие пути.
	path       string
	configPath string

	mu      sync.Mutex
	started bool   // первичный импорт выполнен
	offset  int64  // файл прочитан до этого байта
	pending []byte // незавершённая строка (хвост без '\n')
}

func parseAutolistLog(line string) (logging.Level, string, bool) {
	parts := strings.SplitN(line, " ", 5)
	if len(parts) != 5 {
		return "", "", false
	}
	var level logging.Level
	switch parts[3] {
	case "DEBUG":
		level = logging.LevelDebug
	case "INFO":
		level = logging.LevelInfo
	case "FULL":
		level = logging.LevelFull
	case "WARN":
		level = logging.LevelWarn
	case "ERROR", "FATAL":
		level = logging.LevelError
	default:
		return "", "", false
	}
	return level, parts[0] + " " + parts[1] + " " + parts[2] + " " + parts[4], true
}

func (s *autolistLogSync) debugEnabled() bool {
	path := s.configPath
	if path == "" {
		path = autolistConfigDefaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		JournalDebug bool `json:"journalDebug"`
	}
	return json.Unmarshal(data, &cfg) == nil && cfg.JournalDebug
}

// sync доталкивает файл до текущего конца. В общий журнал уходят только
// полные строки с явным уровнем; DEBUG дополнительно управляется journalDebug.
// action="autolist" — идентификатор источника; дальше события публикует панель.
func (s *autolistLogSync) sync(log *logging.ScopedLogger) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.path
	if path == "" {
		path = autolistLogDefaultPath
	}
	f, err := os.Open(path)
	if err != nil {
		return // файла нет (сателлит не ставился/ещё не писал) — просто нет событий
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	size := fi.Size()

	first := !s.started
	switch {
	case first:
		s.started = true
		s.offset = size - autolistLogImportBytes
		if s.offset < 0 {
			s.offset = 0
		}
	case size < s.offset:
		// RAMLog усекает файл при 5 MiB: всё, что сейчас в файле, новое —
		// читаем актуальный файл с нуля, дублей не будет.
		s.offset = 0
		s.pending = s.pending[:0]
	}
	if size == s.offset {
		return // новых байт нет; незавершённый хвост ждёт в pending
	}

	debugEnabled := s.debugEnabled()
	emit := func(line string) {
		level, message, ok := parseAutolistLog(line)
		if !ok || (level == logging.LevelDebug && !debugEnabled) || log == nil {
			return
		}
		log.At(level, "autolist", "", message)
	}

	chunk := make([]byte, size-s.offset)
	n, err := f.ReadAt(chunk, s.offset)
	if err != nil && err != io.EOF {
		return // прочитать не смогли — позицию не двигаем
	}
	chunk = chunk[:n]
	if first && s.offset > 0 {
		// врезались в середину строки — неполную голову выкидываем
		if i := bytes.IndexByte(chunk, '\n'); i >= 0 {
			chunk = chunk[i+1:]
		} else {
			chunk = nil
		}
	}

	merged := make([]byte, 0, len(s.pending)+len(chunk))
	merged = append(merged, s.pending...)
	merged = append(merged, chunk...)
	var lines []string
	for {
		i := bytes.IndexByte(merged, '\n')
		if i < 0 {
			break
		}
		line := string(merged[:i])
		merged = merged[i+1:]
		if first {
			lines = append(lines, line) // потолок строк применим ниже, к хвосту
		} else {
			emit(line)
		}
	}
	if first {
		if len(lines) > autolistLogImportLines {
			lines = lines[len(lines)-autolistLogImportLines:]
		}
		for _, line := range lines {
			emit(line)
		}
	}
	s.pending = append(s.pending[:0], merged...)
	s.offset += int64(n)
}
