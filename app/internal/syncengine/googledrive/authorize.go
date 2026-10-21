package googledrive

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

type callback struct {
	code string
	err  error
}

// Authorize runs Google's loopback OAuth flow and returns a refresh token.
// It listens on listenAddr and expects the browser to reach it at http://localhost:<port>/.
func Authorize(ctx context.Context, cfg *oauth2.Config, listenAddr string, out io.Writer) (string, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	flow := *cfg
	flow.RedirectURL = fmt.Sprintf("http://localhost:%d/", port)

	state := rand.Text()
	verifier := oauth2.GenerateVerifier()
	authURL := flow.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))

	callbacks := make(chan callback, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			result := callback{code: q.Get("code")}
			switch {
			case q.Get("state") != state:
				result.err = errors.New("state mismatch in OAuth callback")
			case q.Get("error") != "":
				result.err = fmt.Errorf("authorization denied: %s", q.Get("error"))
			case result.code == "":
				result.err = errors.New("no authorization code in OAuth callback")
			}
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
		return "", ctx.Err()
	case result = <-callbacks:
	}
	if result.err != nil {
		return "", result.err
	}

	token, err := flow.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", fmt.Errorf("exchange authorization code: %w", err)
	}
	if token.RefreshToken == "" {
		return "", errors.New("google returned no refresh token; revoke url2supernote at https://myaccount.google.com/permissions and retry")
	}
	return token.RefreshToken, nil
}
