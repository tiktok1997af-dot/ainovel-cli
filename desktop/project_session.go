package main

import (
	"sync"

	"github.com/voocel/ainovel-cli/internal/desktopui"
)

// projectSessionController is the GUI-02C.2 single-runtime serialization
// foundation. It is desktop-internal and deliberately exposes no Wails-bound
// project lifecycle surface. GUI-02C.3 will build typed lifecycle operations
// on top of these leases.
type projectSessionController struct {
	mu      sync.RWMutex
	runtime desktopui.RuntimeClient
}

func newProjectSessionController(runtime desktopui.RuntimeClient) *projectSessionController {
	return &projectSessionController{runtime: runtime}
}

// withRuntime holds the read lease for the full callback. Future project
// lifecycle mutations therefore cannot replace or close the active runtime
// while Snapshot, Query, or Dispatch is still using it.
func (s *projectSessionController) withRuntime(action func(desktopui.RuntimeClient)) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.runtime == nil {
		return false
	}
	action(s.runtime)
	return true
}

// mutate serializes project-session mutation against all runtime readers. The
// callback returns the runtime that should become active. This gate does not
// create, close, or expose project runtimes; those lifecycle semantics remain
// owned by GUI-02C.3.
func (s *projectSessionController) mutate(action func(desktopui.RuntimeClient) desktopui.RuntimeClient) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtime = action(s.runtime)
}
