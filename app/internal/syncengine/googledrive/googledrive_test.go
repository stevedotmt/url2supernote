package googledrive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stevedotmt/url2supernote/app/internal/syncengine"
)

type driveFile struct {
	id, name, parent, mimeType string
}

type fakeDrive struct {
	mu      sync.Mutex
	files   []driveFile
	lists   int
	status  int
	queries []string
}

func (d *fakeDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != 0 {
		w.WriteHeader(d.status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": d.status, "message": "denied"}})
		return
	}
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files"):
		d.lists++
		q := r.URL.Query().Get("q")
		d.queries = append(d.queries, q)
		var matches []map[string]string
		for _, f := range d.files {
			if strings.Contains(q, "name = "+quote(f.name)) && strings.Contains(q, quote(f.parent)+" in parents") &&
				(!strings.Contains(q, "mimeType") || f.mimeType == folderMimeType) {
				matches = append(matches, map[string]string{"id": f.id})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": matches})
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/"):
		body, _ := io.ReadAll(r.Body)
		var meta struct {
			Name    string   `json:"name"`
			Parents []string `json:"parents"`
		}
		start := strings.Index(string(body), "{")
		end := strings.Index(string(body), "}")
		_ = json.Unmarshal(body[start:end+1], &meta)
		d.files = append(d.files, driveFile{id: "new", name: meta.Name, parent: meta.Parents[0], mimeType: epubMimeType})
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new"})
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.String(), http.StatusNotFound)
	}
}

func newTestEngine(t *testing.T, d *fakeDrive, destination string) *Engine {
	t.Helper()
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	client := &http.Client{Transport: rewriteHost{target: target}}
	engine, err := NewWithHTTPClient(context.Background(), client, destination)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

type rewriteHost struct{ target *url.URL }

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func supernoteDrive() *fakeDrive {
	return &fakeDrive{files: []driveFile{
		{id: "sn", name: "Supernote", parent: "root", mimeType: folderMimeType},
		{id: "inbox", name: "INBOX", parent: "sn", mimeType: folderMimeType},
		{id: "f1", name: "Old — Site.epub", parent: "inbox", mimeType: epubMimeType},
	}}
}

func TestEngineExistsAndPut(t *testing.T) {
	d := supernoteDrive()
	engine := newTestEngine(t, d, "/Supernote/INBOX/")

	exists, err := engine.Exists(context.Background(), "Old — Site.epub")
	if err != nil || !exists {
		t.Fatalf("Exists(existing) = %v, %v", exists, err)
	}
	exists, err = engine.Exists(context.Background(), "New — Site.epub")
	if err != nil || exists {
		t.Fatalf("Exists(new) = %v, %v", exists, err)
	}
	if err := engine.Put(context.Background(), "New — Site.epub", []byte("epub")); err != nil {
		t.Fatal(err)
	}

	if last := d.files[len(d.files)-1]; last.name != "New — Site.epub" || last.parent != "inbox" {
		t.Errorf("uploaded %+v", last)
	}
	if d.lists != 4 {
		t.Errorf("made %d list calls, want 4 (Destination resolved once)", d.lists)
	}
}

func TestEngineMissingDestination(t *testing.T) {
	engine := newTestEngine(t, supernoteDrive(), "Supernote/Document/Articles")

	_, err := engine.Exists(context.Background(), "x.epub")

	if !errors.Is(err, syncengine.ErrDestinationNotFound) || !strings.Contains(err.Error(), "Supernote/Document") {
		t.Errorf("err = %v", err)
	}
}

func TestEngineUnauthorized(t *testing.T) {
	d := supernoteDrive()
	d.status = http.StatusUnauthorized
	engine := newTestEngine(t, d, "Supernote/INBOX")

	_, err := engine.Exists(context.Background(), "x.epub")

	if !errors.Is(err, syncengine.ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestEngineEscapesQueries(t *testing.T) {
	d := supernoteDrive()
	engine := newTestEngine(t, d, "Supernote/INBOX")

	if _, err := engine.Exists(context.Background(), `It's a \ test`); err != nil {
		t.Fatal(err)
	}

	if q := d.queries[len(d.queries)-1]; !strings.Contains(q, `name = 'It\'s a \\ test'`) {
		t.Errorf("query = %s", q)
	}
}

func TestNewRejectsEmptyDestination(t *testing.T) {
	if _, err := NewWithHTTPClient(context.Background(), http.DefaultClient, " / "); err == nil {
		t.Error("err = nil")
	}
}
