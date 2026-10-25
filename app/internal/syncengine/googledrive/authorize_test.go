package googledrive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type urlWriter chan string

var authURLPattern = regexp.MustCompile(`https?://\S+`)

func (w urlWriter) Write(p []byte) (int, error) {
	if u := authURLPattern.FindString(string(p)); u != "" {
		w <- u
	}
	return len(p), nil
}

func fakeGoogle(t *testing.T) *oauth2.Config {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	cfg := OAuthConfig("client", "secret")
	cfg.Endpoint = oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL + "/token"}
	return cfg
}

func pickInBrowser(t *testing.T, authURL, picked string) url.Values {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	callback := url.Values{"state": {q.Get("state")}, "code": {"code"}, "picked_file_ids": {picked}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, q.Get("redirect_uri")+"?"+callback.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return q
}

func TestAuthorizeReturnsThePickedFolder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	urls := make(urlWriter, 1)
	type outcome struct {
		token, folder string
		err           error
	}
	done := make(chan outcome, 1)
	go func() {
		token, folder, err := Authorize(ctx, fakeGoogle(t), "127.0.0.1:0", urls)
		done <- outcome{token, folder, err}
	}()

	q := pickInBrowser(t, <-urls, "folder123")
	got := <-done

	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.token != "refresh" || got.folder != "folder123" {
		t.Errorf("got token %q, folder %q", got.token, got.folder)
	}
	for key, want := range map[string]string{
		"scope":                  "https://www.googleapis.com/auth/drive.file",
		"prompt":                 "consent",
		"access_type":            "offline",
		"trigger_onepick":        "true",
		"allow_folder_selection": "true",
		"mimetypes":              folderMimeType,
	} {
		if q.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, q.Get(key), want)
		}
	}
}

func TestAuthorizeRequiresExactlyOneFolder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	urls := make(urlWriter, 1)
	done := make(chan error, 1)
	go func() {
		_, _, err := Authorize(ctx, fakeGoogle(t), "127.0.0.1:0", urls)
		done <- err
	}()

	pickInBrowser(t, <-urls, "a,b")

	if err := <-done; err == nil || !strings.Contains(err.Error(), "exactly one folder") {
		t.Errorf("err = %v", err)
	}
}

func TestParseCallbackRejectsAnythingButOneFolderID(t *testing.T) {
	for _, picked := range []string{"", "a,b", "a,", "not/an/id"} {
		got := parseCallback(url.Values{"state": {"s"}, "code": {"c"}, "picked_file_ids": {picked}}, "s")
		if got.err == nil || !strings.Contains(got.err.Error(), "exactly one folder") {
			t.Errorf("picked %q: err = %v", picked, got.err)
		}
	}
}
