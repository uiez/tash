package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-zglob"
)

type watcher struct {
	w   *fsnotify.Watcher
	log indentLogger

	watching     map[string]bool
	dirPatterns  []func(string) bool
	filePatterns []func(string) bool
}

func newWatcher(log indentLogger, dirs, files []string) (*watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher failed: %w", err)
	}
	w := watcher{
		log:      log,
		w:        fsWatcher,
		watching: make(map[string]bool),
	}
	watchDirs := make(map[string]bool)
	for _, d := range dirs {
		matched, err := zglob.Glob(d)
		if err != nil {
			return nil, fmt.Errorf("glob matching dir failed: %s, %w", d, err)
		}
		for _, m := range matched {
			path := filepath.Clean(m)
			stat, err := os.Stat(path)
			if err != nil || !stat.IsDir() {
				continue
			}
			watchDirs[path] = true
		}
		e, err := zglob.New(d)
		if err != nil {
			return nil, fmt.Errorf("compile dir pattern failed: %s, %w", d, err)
		}
		w.dirPatterns = append(w.dirPatterns, e.Match)
	}
	for _, pattern := range files {
		e, err := zglob.New(pattern)
		if err != nil {
			return nil, fmt.Errorf("compile file pattern failed: %s, %w", pattern, err)
		}
		w.filePatterns = append(w.filePatterns, e.Match)
	}

	dirs = dirs[:0]
	for dir := range watchDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		w.watchDir(dir, true)
	}
	return &w, nil
}

func (w *watcher) close() {
	_ = w.w.Close()
}

func (w *watcher) run(notify func()) {
	var debouncer *time.Timer
	var debouncerC <-chan time.Time
	startTimerIfNeeded := func() {
		if debouncerC == nil {
			if debouncer == nil {
				debouncer = time.NewTimer(time.Second)
			} else {
				debouncer.Reset(time.Second)
			}
			debouncerC = debouncer.C
		}
	}
	clearTimer := func() {
		if debouncerC != nil {
			debouncer.Stop()
			debouncerC = nil
		}
	}
	defer clearTimer()

	for {
		var shouldNotify bool
	OUTER:
		select {
		case err := <-w.w.Errors:
			if err != nil {
				w.log.warnln("watcher reported error:", err)
			}
		case event := <-w.w.Events:
			switch {
			case event.Op&fsnotify.Create != 0:
				if !w.shouldWatchPath(event.Name, nil) {
					break OUTER
				}
				w.watchPath(event.Name)
			case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0 || event.Op&fsnotify.Write != 0:
				isDir, has := w.watching[event.Name]
				if has {
					_ = w.w.Remove(event.Name)
				}
				if !has || isDir {
					break OUTER
				}
			default:
				break OUTER
			}
			shouldNotify = true
		case <-debouncerC:
			notify()
			clearTimer()
		}

		if !shouldNotify {
			continue
		}
		startTimerIfNeeded()
	}
}

func (w *watcher) shouldWatchPath(path string, isDir *bool) bool {
	var dir bool
	if isDir != nil {
		dir = *isDir
	} else {
		stat, err := os.Stat(path)
		if err != nil {
			return false
		}
		dir = stat.IsDir()
	}
	pats := w.filePatterns
	if dir {
		pats = w.dirPatterns
	}
	for _, p := range pats {
		if p(path) {
			return true
		}
	}
	return false
}

func (w *watcher) watchPath(path string) {
	stat, err := os.Stat(path)
	if err != nil {
		w.log.warnln("retrieve file stat failed:", path, err)
		return
	}
	isDir := stat.IsDir()
	if !w.shouldWatchPath(path, &isDir) {
		return
	}
	if isDir {
		w.watchDir(path, false)
	} else {
		w.watchFile(path)
	}
}

func (w *watcher) watchFile(path string) {
	w.log.debugln("add watch file:", path)
	err := w.w.Add(path)
	if err != nil {
		w.log.warnln("watch file failed:", path, err)
	} else {
		w.watching[path] = false
	}
}

func (w *watcher) watchDir(dir string, direntsExcludeDir bool) {
	w.log.debugln("add watch dir:", dir)
	err := w.w.Add(dir)
	if err != nil {
		w.log.warnln("add watch dir failed:", dir, err)
	} else {
		w.watching[dir] = true
	}

	dirents, err := ioutil.ReadDir(dir)
	if err != nil {
		w.log.warnln("list watch dir items failed:", dir, err)
	}
	for _, ent := range dirents {
		path := filepath.Join(dir, ent.Name())
		if direntsExcludeDir && ent.IsDir() {
			continue
		}
		w.watchPath(path)
	}
}
