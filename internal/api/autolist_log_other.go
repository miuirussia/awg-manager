//go:build !linux

package api

import (
	"sync"
	"time"
)

type otherLogWatcher struct {
	path     string
	onNotify func()
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
}

func newAutolistLogWatcher(path string, onNotify func()) autolistLogWatcher {
	return &otherLogWatcher{
		path:     path,
		onNotify: onNotify,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (w *otherLogWatcher) start() error {
	if w.onNotify != nil {
		w.onNotify()
	}
	go func() {
		defer close(w.doneCh)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				if w.onNotify != nil {
					w.onNotify()
				}
			}
		}
	}()
	return nil
}

func (w *otherLogWatcher) stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		<-w.doneCh
	})
}
