package googledrive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/stevedotmt/url2supernote/app/internal/syncengine"
)

type driveFile struct {
	id, name, parent, mimeType string
}

type fakeDrive struct {
	mu      sync.Mutex
	files   []driveFile
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
		q := r.URL.Query().Get("q")
		d.queries = append(d.queries, q)
		var matches []map[string]string
		for _, f := range d.files {
			if strings.Contains(q, "name = "+quote(f.name)) && strings.Contains(q, quote(f.parent)+" in parents") {
				matches = append(matches, map[string]string{"id": f.id})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": matches})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/"):
		id := path.Base(r.URL.Path)
		for _, f := range d.files {
			if f.id == id {
				_ = json.NewEncoder(w).Encode(map[string]string{"name": f.name, "mimeType": f.mimeType})
				return
			}
		}
		notFound(w)
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/"):
		body, _ := io.ReadAll(r.Body)
		var meta struct {
			Name    string   `json:"name"`
			Parents []string `json:"parents"`
		}
		start := strings.Index(string(body), "{")
		end := strings.Index(string(body), "}")
		_ = json.Unmarshal(body[start:end+1], &meta)
		if len(meta.Parents) != 1 || !d.has(meta.Parents[0]) {
			notFound(w)
			return
		}
		d.files = append(d.files, driveFile{id: "new", name: meta.Name, parent: meta.Parents[0], mimeType: epubMimeType})
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new"})
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.String(), http.StatusNotFound)
	}
}

func (d *fakeDrive) has(id string) bool {
	for _, f := range d.files {
		if f.id == id {
			return true
		}
	}
	return false
}

func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": http.StatusNotFound, "message": "File not found"}})
}

func driveClient(t *testing.T, d *fakeDrive) *http.Client {
	t.Helper()
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	return &http.Client{Transport: rewriteHost{target: target}}
}

func tokenWithScope(scope string) oauth2.TokenSource {
	token := (&oauth2.Token{AccessToken: "a"}).WithExtra(map[string]any{"scope": scope})
	return oauth2.StaticTokenSource(token)
}

func openEngine(d *fakeDrive, t *testing.T, folderID string) (*Engine, error) {
	t.Helper()
	return open(context.Background(), driveClient(t, d), tokenWithScope(drive.DriveFileScope), folderID)
}

func newTestEngine(t *testing.T, d *fakeDrive, folderID string) *Engine {
	t.Helper()
	engine, err := openEngine(d, t, folderID)
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
		{id: "inbox", name: "INBOX", parent: "sn", mimeType: folderMimeType},
		{id: "f1", name: "Old — Site.epub", parent: "inbox", mimeType: epubMimeType},
	}}
}

func TestEngineExistsAndPut(t *testing.T) {
	d := supernoteDrive()
	engine := newTestEngine(t, d, "inbox")

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
}

func TestOpenReadsTheFolderName(t *testing.T) {
	if got := newTestEngine(t, supernoteDrive(), "inbox").Name(); got != "INBOX" {
		t.Errorf("Name = %q", got)
	}
}

func TestOpenMissingDestination(t *testing.T) {
	if _, err := openEngine(supernoteDrive(), t, "deleted"); !errors.Is(err, syncengine.ErrDestinationNotFound) {
		t.Errorf("err = %v, want ErrDestinationNotFound", err)
	}
}

func TestPutIntoMissingDestination(t *testing.T) {
	engine := newTestEngine(t, supernoteDrive(), "inbox")
	engine.folderID = "deleted"

	if err := engine.Put(context.Background(), "x.epub", []byte("epub")); !errors.Is(err, syncengine.ErrDestinationNotFound) {
		t.Errorf("Put err = %v, want ErrDestinationNotFound", err)
	}
}

func TestOpenRejectsAFile(t *testing.T) {
	if _, err := openEngine(supernoteDrive(), t, "f1"); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("err = %v", err)
	}
}

func TestEngineUnauthorized(t *testing.T) {
	d := supernoteDrive()
	engine := newTestEngine(t, d, "inbox")
	d.status = http.StatusUnauthorized

	_, err := engine.Exists(context.Background(), "x.epub")

	if !errors.Is(err, syncengine.ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestEngineEscapesQueries(t *testing.T) {
	d := supernoteDrive()
	engine := newTestEngine(t, d, "inbox")

	if _, err := engine.Exists(context.Background(), `It's a \ test`); err != nil {
		t.Fatal(err)
	}

	if q := d.queries[len(d.queries)-1]; !strings.Contains(q, `name = 'It\'s a \\ test'`) {
		t.Errorf("query = %s", q)
	}
}

func TestOpenRejectsMalformedFolderID(t *testing.T) {
	for _, id := range []string{"", " ", "Supernote/INBOX"} {
		if _, err := openEngine(supernoteDrive(), t, id); err == nil {
			t.Errorf("Open(%q) err = nil", id)
		}
	}
}

func TestOpenChecksTheGrantedScope(t *testing.T) {
	client := driveClient(t, supernoteDrive())

	_, err := open(context.Background(), client, tokenWithScope(drive.DriveFileScope), "inbox")
	if err != nil {
		t.Errorf("drive.file token rejected: %v", err)
	}
	for _, scope := range []string{drive.DriveScope, drive.DriveFileScope + " " + drive.DriveScope, ""} {
		_, err := open(context.Background(), client, tokenWithScope(scope), "inbox")
		if !errors.Is(err, ErrBroadScope) {
			t.Errorf("scope %q: err = %v, want ErrBroadScope", scope, err)
		}
	}
}

func TestClassifyTellsRateLimitsFromRejectedAuthorization(t *testing.T) {
	limited := &googleapi.Error{Code: http.StatusForbidden, Errors: []googleapi.ErrorItem{{Reason: "rateLimitExceeded"}}}
	if err := classify(limited); errors.Is(err, syncengine.ErrUnauthorized) {
		t.Errorf("rate limit classified as unauthorized: %v", err)
	}
	denied := &googleapi.Error{Code: http.StatusForbidden, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}}
	if err := classify(denied); !errors.Is(err, syncengine.ErrUnauthorized) {
		t.Errorf("forbidden = %v, want ErrUnauthorized", err)
	}
}
