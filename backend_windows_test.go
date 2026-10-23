//go:build windows

package fsnotify

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRemoveState(t *testing.T) {
	// TODO: the Windows backend is too confusing; needs some serious attention.
	t.Skip("broken test")

	var (
		tmp  = t.TempDir()
		dir  = join(tmp, "dir")
		file = join(dir, "file")
	)
	mkdir(t, dir)
	touch(t, file)

	w := newWatcher(t, tmp)
	addWatch(t, w, tmp)
	addWatch(t, w, file)

	check := func(want int) {
		t.Helper()
		if len(w.b.(*readDirChangesW).watches) != want {
			var d []string
			for k, v := range w.b.(*readDirChangesW).watches {
				d = append(d, fmt.Sprintf("%#v = %#v", k, v))
			}
			t.Errorf("unexpected number of entries in w.watches (have %d, want %d):\n%v",
				len(w.b.(*readDirChangesW).watches), want, strings.Join(d, "\n"))
		}
	}

	check(2)

	// Shouldn't change internal state.
	if err := w.Add("/path-doesnt-exist"); err == nil {
		t.Fatal(err)
	}
	check(2)

	if err := w.Remove(file); err != nil {
		t.Fatal(err)
	}
	check(1)

	if err := w.Remove(tmp); err != nil {
		t.Fatal(err)
	}
	check(0)
}

func TestWindowsRemWatch(t *testing.T) {
	tmp := t.TempDir()

	touch(t, tmp, "file")

	w := newWatcher(t)
	defer w.Close()

	addWatch(t, w, tmp)
	if err := w.Remove(tmp); err != nil {
		t.Fatalf("Could not remove the watch: %v\n", err)
	}
	if err := w.b.(*readDirChangesW).remWatch(tmp); err == nil {
		t.Fatal("Should be fail with closed handle\n")
	}
}

// sendError on a closing watcher must leave Close's reply channel queued for
// the reader. If it is dropped, the reader never runs the shutdown cleanup and
// Close waits on the reply forever.
func TestSendErrorKeepsCloseReply(t *testing.T) {
	w := &readDirChangesW{
		Errors: make(chan error), // nobody reads errors any more
		done:   make(chan chan<- error, 1),
	}
	reply := make(chan error)
	w.done <- reply

	if w.sendError(errors.New("overflow")) {
		t.Fatal("sendError reported the error as delivered")
	}
	select {
	case got := <-w.done:
		if got != reply {
			t.Fatal("sendError replaced Close's reply channel")
		}
	default:
		t.Fatal("sendError dropped Close's reply channel")
	}
}

// Close must return when the error consumer has gone away and the backend
// then reports an error: the reader keeps the reply and completes shutdown.
func TestCloseAfterUnreadError(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	b := w.b.(*readDirChangesW)

	// Close queues its reply and wakes the reader; an error raised before the
	// reader takes it meets an Errors channel nobody reads.
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	reply := make(chan error)
	b.done <- reply
	if b.sendError(errors.New("overflow")) {
		t.Fatal("sendError reported the error as delivered")
	}
	if err := b.wakeupReader(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-reply:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the reader never completed shutdown after an undelivered error")
	}
}
