package desk

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeniorAlfred/macron-agent/pkg/app"
	"github.com/SeniorAlfred/macron-agent/pkg/updater"
)

type UpdateStatus struct {
	Current    string `json:"current"`
	Latest     string `json:"latest,omitempty"`
	Tag        string `json:"tag,omitempty"`
	URL        string `json:"url,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Available  bool   `json:"available"`
	Installing bool   `json:"installing,omitempty"`
	Installed  bool   `json:"installed,omitempty"`
	Step       string `json:"step,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  int64  `json:"checkedAt,omitempty"`
}

type updateState struct {
	mu         sync.Mutex
	rel        *updater.Release
	status     UpdateStatus
	checkedAt  time.Time
	installing bool
}

const updateCacheTTL = 30 * time.Minute

func (s *Server) checkUpdate(ctx context.Context, force bool) UpdateStatus {
	s.upd.mu.Lock()
	if s.upd.installing {
		st := s.upd.status
		s.upd.mu.Unlock()
		return st
	}
	if !force && !s.upd.checkedAt.IsZero() && time.Since(s.upd.checkedAt) < updateCacheTTL {
		st := s.upd.status
		s.upd.mu.Unlock()
		return st
	}
	s.upd.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rel, err := updater.Latest(cctx, "")

	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	now := time.Now()
	st := UpdateStatus{
		Current:   app.Version,
		CheckedAt: now.UnixMilli(),
	}
	if err != nil {
		st.Error = err.Error()
		s.upd.status = st
		return st
	}
	latest := rel.Version()
	st.Latest = latest
	st.Tag = rel.TagName
	st.URL = rel.HTMLURL
	st.Notes = clipRunes(strings.TrimSpace(rel.Body), 2000)
	st.Available = updater.Newer(latest, app.Version)
	s.upd.rel = rel
	s.upd.status = st
	s.upd.checkedAt = now
	return st
}

func (s *Server) updateStatusAPI(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1"
	st := s.checkUpdate(r.Context(), force)
	writeJSON(w, st)
}

func (s *Server) updateInstallAPI(w http.ResponseWriter, r *http.Request) {
	s.upd.mu.Lock()
	if s.upd.installing {
		st := s.upd.status
		s.upd.mu.Unlock()
		writeJSON(w, st)
		return
	}
	rel := s.upd.rel
	s.upd.installing = true
	s.upd.status.Installing = true
	s.upd.status.Error = ""
	s.upd.status.Step = "checking release"
	s.upd.mu.Unlock()

	publish := func(st UpdateStatus) {
		if s.hub != nil {
			s.hub.publish(map[string]any{"type": "update", "update": st})
		}
	}

	if rel == nil {
		cctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		var err error
		rel, err = updater.Latest(cctx, "")
		cancel()
		if err != nil {
			s.upd.mu.Lock()
			s.upd.installing = false
			s.upd.status.Installing = false
			s.upd.status.Error = err.Error()
			st := s.upd.status
			s.upd.mu.Unlock()
			publish(st)
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}

	if !updater.Newer(rel.Version(), app.Version) {
		s.upd.mu.Lock()
		s.upd.installing = false
		s.upd.status = UpdateStatus{
			Current:   app.Version,
			Latest:    rel.Version(),
			Tag:       rel.TagName,
			URL:       rel.HTMLURL,
			Available: false,
			CheckedAt: time.Now().UnixMilli(),
		}
		st := s.upd.status
		s.upd.mu.Unlock()
		publish(st)
		writeJSON(w, st)
		return
	}

	uctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := updater.Install(uctx, rel, func(step string) {
		s.upd.mu.Lock()
		s.upd.status.Step = step
		st := s.upd.status
		s.upd.mu.Unlock()
		publish(st)
	})
	if err != nil {
		s.upd.mu.Lock()
		s.upd.installing = false
		s.upd.status.Installing = false
		s.upd.status.Step = ""
		s.upd.status.Error = err.Error()
		st := s.upd.status
		s.upd.mu.Unlock()
		publish(st)
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	s.upd.mu.Lock()
	s.upd.installing = false
	s.upd.status = UpdateStatus{
		Current:   app.Version,
		Latest:    rel.Version(),
		Tag:       rel.TagName,
		URL:       rel.HTMLURL,
		Available: false,
		Installed: true,
		Step:      "installed",
		CheckedAt: time.Now().UnixMilli(),
	}
	st := s.upd.status
	s.upd.mu.Unlock()
	publish(st)
	writeJSON(w, st)
}
