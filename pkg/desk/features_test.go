package desk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeniorAlfred/macron-agent/pkg/agent"
	"github.com/SeniorAlfred/macron-agent/pkg/updater"
)

type fakeSession struct {
	dir     string
	thread  string
	newHits int
}

func (f *fakeSession) Turn(context.Context, string, *agent.EventListener, func() []string, func() <-chan struct{}) (*agent.Result, error) {
	return &agent.Result{Answer: "ok"}, nil
}
func (f *fakeSession) New(context.Context) error               { f.newHits++; return nil }
func (f *fakeSession) Compact(context.Context) (string, error) { return "", nil }
func (f *fakeSession) SetMode(string) error                    { return nil }
func (f *fakeSession) SetThread(tid string)                    { f.thread = tid }
func (f *fakeSession) FilePath(name string) (string, fs.FileInfo, error) {
	p := filepath.Join(f.dir, filepath.FromSlash(name))
	info, err := os.Stat(p)
	return p, info, err
}

type testFeaturesHost struct {
	Host
	dataDir string
	projDir string
	sess    *fakeSession

	memories []MemoryEntry
	skills   []SkillView
	conns    []ConnectionDetail
	savedEnv map[string]string
}

func (h *testFeaturesHost) Info() Info                     { return Info{Version: "1.16.0", Platform: "linux"} }
func (h *testFeaturesHost) DataDir() string                { return h.dataDir }
func (h *testFeaturesHost) DefaultProject() string         { return h.projDir }
func (h *testFeaturesHost) Projects() []string             { return nil }
func (h *testFeaturesHost) SaveProjects([]string) error    { return nil }
func (h *testFeaturesHost) Servers() []ServerSpec          { return nil }
func (h *testFeaturesHost) SaveServers([]ServerSpec) error { return nil }
func (h *testFeaturesHost) Open(dir string) (Session, error) {
	if h.sess == nil {
		h.sess = &fakeSession{dir: dir}
	}
	return h.sess, nil
}
func (h *testFeaturesHost) Memories() ([]MemoryEntry, error) { return h.memories, nil }
func (h *testFeaturesHost) AddMemory(scope, text string) (MemoryEntry, error) {
	e := MemoryEntry{ID: len(h.memories) + 1, Scope: scope, Text: text}
	h.memories = append(h.memories, e)
	return e, nil
}
func (h *testFeaturesHost) DeleteMemory(scope string, id int) error {
	for i, m := range h.memories {
		if m.ID == id && m.Scope == scope {
			h.memories = append(h.memories[:i], h.memories[i+1:]...)
			return nil
		}
	}
	return nil
}
func (h *testFeaturesHost) Skills() []SkillView { return h.skills }
func (h *testFeaturesHost) SetSkillEnabled(name string, on bool) error {
	for i := range h.skills {
		if h.skills[i].Name == name {
			h.skills[i].Enabled = on
		}
	}
	return nil
}
func (h *testFeaturesHost) DeleteSkill(name string) error {
	for i := range h.skills {
		if h.skills[i].Name == name {
			h.skills = append(h.skills[:i], h.skills[i+1:]...)
			return nil
		}
	}
	return nil
}
func (h *testFeaturesHost) ConnectionDetails() []ConnectionDetail { return h.conns }
func (h *testFeaturesHost) SaveConnectionValues(v map[string]string) error {
	h.savedEnv = v
	return nil
}

func newFeatureTestServer(t *testing.T) (*Server, *testFeaturesHost, string) {
	t.Helper()
	dataDir := t.TempDir()
	projDir := t.TempDir()
	h := &testFeaturesHost{
		dataDir: dataDir,
		projDir: projDir,
		skills:  []SkillView{{Name: "code-review", Category: "dev", Description: "Review code", Enabled: true}},
		conns:   []ConnectionDetail{{ID: "github", Name: "GitHub", Fields: []ConnectionField{{Key: "GITHUB_TOKEN", Secret: true}}}},
	}
	s := New(h)
	pid := projectID(projDir)
	return s, h, pid
}

func authedReq(s *Server, method, target string, body []byte, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+s.Token())
	req.Header.Set("X-Macron", "1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestUploadAndFilesAPI(t *testing.T) {
	s, h, pid := newFeatureTestServer(t)

	// 1. Multipart upload
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "../evil/screen shot.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("fake-png-bytes"))
	_ = mw.Close()

	w := authedReq(s, http.MethodPost, "/api/projects/"+pid+"/upload", buf.Bytes(), mw.FormDataContentType())
	if w.Code != http.StatusOK {
		t.Fatalf("multipart upload status %d: %s", w.Code, w.Body.String())
	}
	var uploaded File
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uploaded.Name, ".macron-uploads/") || strings.Contains(uploaded.Name, "..") {
		t.Fatalf("unexpected uploaded path: %q", uploaded.Name)
	}

	// 2. JSON base64 upload
	payload, _ := json.Marshal(map[string]string{
		"name": "notes.txt",
		"data": "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("hello desktop")),
	})
	w2 := authedReq(s, http.MethodPost, "/api/projects/"+pid+"/upload", payload, "application/json")
	if w2.Code != http.StatusOK {
		t.Fatalf("json upload status %d: %s", w2.Code, w2.Body.String())
	}

	// 3. Project files search (@mention) skips .macron-uploads and finds project files
	_ = os.MkdirAll(filepath.Join(h.projDir, "pkg", "demo"), 0o755)
	_ = os.WriteFile(filepath.Join(h.projDir, "pkg", "demo", "alpha.go"), []byte("package demo"), 0o644)
	_ = os.WriteFile(filepath.Join(h.projDir, "README.md"), []byte("# demo"), 0o644)

	wf := authedReq(s, http.MethodGet, "/api/projects/"+pid+"/files?q=alpha", nil, "")
	if wf.Code != http.StatusOK {
		t.Fatalf("files API status %d: %s", wf.Code, wf.Body.String())
	}
	var fres struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(wf.Body.Bytes(), &fres); err != nil {
		t.Fatal(err)
	}
	if len(fres.Files) != 1 || fres.Files[0] != "pkg/demo/alpha.go" {
		t.Fatalf("unexpected files result: %+v", fres.Files)
	}
}

func TestGitChangesDiffAndRevertAPI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	s, h, pid := newFeatureTestServer(t)

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", h.projDir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	_ = os.WriteFile(filepath.Join(h.projDir, "app.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	git("add", "app.go")
	git("commit", "-m", "init")

	// Modify tracked file + add untracked file
	_ = os.WriteFile(filepath.Join(h.projDir, "app.go"), []byte("package main\nfunc main() { println(1) }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(h.projDir, "extra.txt"), []byte("new line\n"), 0o644)

	wc := authedReq(s, http.MethodGet, "/api/projects/"+pid+"/changes", nil, "")
	if wc.Code != http.StatusOK {
		t.Fatalf("changes status %d: %s", wc.Code, wc.Body.String())
	}
	var cres struct {
		Git    bool        `json:"git"`
		Branch string      `json:"branch"`
		Files  []GitChange `json:"files"`
	}
	if err := json.Unmarshal(wc.Body.Bytes(), &cres); err != nil {
		t.Fatal(err)
	}
	if !cres.Git || cres.Branch != "main" || len(cres.Files) != 2 {
		t.Fatalf("unexpected changes: %+v", cres)
	}

	// Diff tracked file
	wd := authedReq(s, http.MethodGet, "/api/projects/"+pid+"/diff?path=app.go", nil, "")
	if wd.Code != http.StatusOK || !strings.Contains(wd.Body.String(), "println(1)") {
		t.Fatalf("unexpected diff response (%d): %s", wd.Code, wd.Body.String())
	}

	// Revert tracked file
	wr := authedReq(s, http.MethodPost, "/api/projects/"+pid+"/revert", []byte(`{"path":"app.go"}`), "application/json")
	if wr.Code != http.StatusOK {
		t.Fatalf("revert status %d: %s", wr.Code, wr.Body.String())
	}
	raw, _ := os.ReadFile(filepath.Join(h.projDir, "app.go"))
	if strings.Contains(string(raw), "println(1)") {
		t.Fatalf("expected app.go to be reverted, got: %s", raw)
	}
}

func TestThreadsLifecycleAPI(t *testing.T) {
	s, h, pid := newFeatureTestServer(t)
	p := s.projects[pid]
	p.mu.Lock()
	p.loadLocked()
	p.appendLocked(Block{Kind: "user", Text: "First thread prompt"})
	firstID := p.activeThread
	p.mu.Unlock()

	// Create new thread via /new
	wn := authedReq(s, http.MethodPost, "/api/projects/"+pid+"/new", nil, "")
	if wn.Code != http.StatusOK {
		t.Fatalf("/new status %d: %s", wn.Code, wn.Body.String())
	}
	var nres struct {
		Thread ThreadSummary `json:"thread"`
	}
	if err := json.Unmarshal(wn.Body.Bytes(), &nres); err != nil {
		t.Fatal(err)
	}
	if nres.Thread.ID == "" || nres.Thread.ID == firstID {
		t.Fatalf("expected a new thread id, got %q (first %q)", nres.Thread.ID, firstID)
	}
	if h.sess == nil || h.sess.thread != nres.Thread.ID {
		t.Fatalf("expected session SetThread(%q), got %+v", nres.Thread.ID, h.sess)
	}

	// Switch back to first thread
	ws := authedReq(s, http.MethodPost, "/api/projects/"+pid+"/threads/"+firstID+"/switch", nil, "")
	if ws.Code != http.StatusOK || !strings.Contains(ws.Body.String(), "First thread prompt") {
		t.Fatalf("switch thread failed (%d): %s", ws.Code, ws.Body.String())
	}

	// Delete second thread
	wd := authedReq(s, http.MethodDelete, "/api/projects/"+pid+"/threads/"+nres.Thread.ID, nil, "")
	if wd.Code != http.StatusOK {
		t.Fatalf("delete thread failed (%d): %s", wd.Code, wd.Body.String())
	}
}

func TestManageAndUpdateAPIs(t *testing.T) {
	s, h, _ := newFeatureTestServer(t)

	// Memory add / list / delete
	wa := authedReq(s, http.MethodPost, "/api/memories", []byte(`{"scope":"memory","text":"Prefer Go 1.26"}`), "application/json")
	if wa.Code != http.StatusOK {
		t.Fatalf("add memory status %d: %s", wa.Code, wa.Body.String())
	}
	wl := authedReq(s, http.MethodGet, "/api/memories", nil, "")
	if wl.Code != http.StatusOK || !strings.Contains(wl.Body.String(), "Prefer Go 1.26") {
		t.Fatalf("list memories unexpected: %s", wl.Body.String())
	}
	wd := authedReq(s, http.MethodDelete, "/api/memories/1?scope=memory", nil, "")
	if wd.Code != http.StatusOK {
		t.Fatalf("delete memory status %d", wd.Code)
	}

	// Skills toggle
	ws := authedReq(s, http.MethodPost, "/api/skills/code-review/enable", []byte(`{"on":false}`), "application/json")
	if ws.Code != http.StatusOK || h.skills[0].Enabled {
		t.Fatalf("toggle skill failed (%d): enabled=%v", ws.Code, h.skills[0].Enabled)
	}

	// Connections save
	wc := authedReq(s, http.MethodPost, "/api/connections", []byte(`{"values":{"GITHUB_TOKEN":"ghp_test"}}`), "application/json")
	if wc.Code != http.StatusOK || h.savedEnv["GITHUB_TOKEN"] != "ghp_test" {
		t.Fatalf("save connection failed (%d): %+v", wc.Code, h.savedEnv)
	}

	// GitHub update check against mock release server
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/SeniorAlfred/Macron-Agent/releases/tag/v9.9.9","body":"New release notes"}`))
	}))
	defer gh.Close()
	origAPI := updater.ReleaseAPI
	updater.ReleaseAPI = gh.URL
	defer func() { updater.ReleaseAPI = origAPI }()

	wu := authedReq(s, http.MethodGet, "/api/update?force=1", nil, "")
	if wu.Code != http.StatusOK {
		t.Fatalf("update status %d: %s", wu.Code, wu.Body.String())
	}
	var ures UpdateStatus
	if err := json.Unmarshal(wu.Body.Bytes(), &ures); err != nil {
		t.Fatal(err)
	}
	if !ures.Available || ures.Latest != "9.9.9" {
		t.Fatalf("expected available update 9.9.9, got %+v", ures)
	}
}
