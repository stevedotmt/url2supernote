package googledrive

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

type callback struct {
	code     string
	folderID string
	err      error
}

// Authorize runs Google's loopback OAuth flow with the Google Picker, so the Owner grants access to the one
// Destination folder they pick rather than their whole Drive. It returns the refresh token and the picked
// folder's ID. It listens on listenAddr and expects the browser to reach it at http://localhost:<port>/.
func Authorize(ctx context.Context, cfg *oauth2.Config, listenAddr string, out io.Writer) (string, string, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return "", "", fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	flow := *cfg
	flow.RedirectURL = fmt.Sprintf("http://localhost:%d/", port)

	state := rand.Text()
	verifier := oauth2.GenerateVerifier()
	authURL := flow.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("trigger_onepick", "true"),
		oauth2.SetAuthURLParam("allow_folder_selection", "true"),
		oauth2.SetAuthURLParam("mimetypes", folderMimeType),
	)

	callbacks := make(chan callback, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			result := parseCallback(r.URL.Query(), state)
			if result.err != nil {
				http.Error(w, result.err.Error(), http.StatusBadRequest)
			} else {
				fmt.Fprintln(w, "url2supernote is authorized. You can close this tab.")
			}
			select {
			case callbacks <- result:
			default:
			}
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	fmt.Fprintf(out, "Open this URL in your browser and allow access to Google Drive:\n\n%s\n\n", authURL)

	var result callback
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case result = <-callbacks:
	}
	if result.err != nil {
		return "", "", result.err
	}

	token, err := flow.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", "", fmt.Errorf("exchange authorization code: %w", err)
	}
	if token.RefreshToken == "" {
		return "", "", errors.New("google returned no refresh token; revoke url2supernote at https://myaccount.google.com/permissions and retry")
	}
	return token.RefreshToken, result.folderID, nil
}

func parseCallback(q url.Values, state string) callback {
	if q.Get("state") != state {
		return callback{err: errors.New("state mismatch in OAuth callback")}
	}
	if denied := q.Get("error"); denied != "" {
		return callback{err: fmt.Errorf("authorization denied: %s", denied)}
	}
	code := q.Get("code")
	if code == "" {
		return callback{err: errors.New("no authorization code in OAuth callback")}
	}
	picked := strings.Split(q.Get("picked_file_ids"), ",")
	if len(picked) != 1 || !ValidFolderID(picked[0]) {
		return callback{err: fmt.Errorf("select exactly one folder in the Google Picker, got %q", q.Get("picked_file_ids"))}
	}
	return callback{code: code, folderID: picked[0]}
}
