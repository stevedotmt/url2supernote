// Package googledrive is the Google Drive Sync Engine.
package googledrive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/stevedotmt/url2supernote/app/internal/syncengine"
)

const (
	folderMimeType = "application/vnd.google-apps.folder"
	epubMimeType   = "application/epub+zip"
)

// OAuthConfig returns the OAuth client configuration for the Owner's own Google Cloud OAuth client.
// Full Drive access is required because the Destination folder is created by the Supernote, not by us.
func OAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveScope},
	}
}

// Engine stores files in a Destination folder on Google Drive.
type Engine struct {
	files       *drive.FilesService
	destination []string

	mu       sync.Mutex
	folderID string
}

var _ syncengine.Engine = (*Engine)(nil)

// New returns an Engine for the Destination path, e.g. "Supernote/INBOX", relative to My Drive.
func New(ctx context.Context, cfg *oauth2.Config, refreshToken, destination string) (*Engine, error) {
	client := cfg.Client(ctx, &oauth2.Token{RefreshToken: refreshToken})
	return NewWithHTTPClient(ctx, client, destination)
}

// NewWithHTTPClient returns an Engine that talks to Drive through an already authorized client.
func NewWithHTTPClient(ctx context.Context, client *http.Client, destination string) (*Engine, error) {
	segments := splitPath(destination)
	if len(segments) == 0 {
		return nil, errors.New("destination must name at least one folder")
	}
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create drive client: %w", err)
	}
	return &Engine{files: service.Files, destination: segments}, nil
}

func (e *Engine) Exists(ctx context.Context, name string) (bool, error) {
	folderID, err := e.folder(ctx)
	if err != nil {
		return false, err
	}
	list, err := e.files.List().
		Q(fmt.Sprintf("name = %s and %s in parents and trashed = false", quote(name), quote(folderID))).
		Fields("files(id)").
		PageSize(1).
		Context(ctx).
		Do()
	if err != nil {
		return false, classify(err)
	}
	return len(list.Files) > 0, nil
}

func (e *Engine) Put(ctx context.Context, name string, content []byte) error {
	folderID, err := e.folder(ctx)
	if err != nil {
		return err
	}
	_, err = e.files.Create(&drive.File{Name: name, Parents: []string{folderID}, MimeType: epubMimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(epubMimeType)).
		Fields("id").
		Context(ctx).
		Do()
	return classify(err)
}

func (e *Engine) folder(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.folderID != "" {
		return e.folderID, nil
	}

	parent := "root"
	for i, segment := range e.destination {
		list, err := e.files.List().
			Q(fmt.Sprintf("name = %s and %s in parents and mimeType = %s and trashed = false",
				quote(segment), quote(parent), quote(folderMimeType))).
			Fields("files(id)").
			OrderBy("createdTime").
			PageSize(1).
			Context(ctx).
			Do()
		if err != nil {
			return "", classify(err)
		}
		if len(list.Files) == 0 {
			return "", fmt.Errorf("%w: %s", syncengine.ErrDestinationNotFound, strings.Join(e.destination[:i+1], "/"))
		}
		parent = list.Files[0].Id
	}

	e.folderID = parent
	return parent, nil
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return fmt.Errorf("%w: %w", syncengine.ErrUnauthorized, err)
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && (apiErr.Code == http.StatusUnauthorized || apiErr.Code == http.StatusForbidden) {
		return fmt.Errorf("%w: %w", syncengine.ErrUnauthorized, err)
	}
	return err
}

func splitPath(path string) []string {
	var segments []string
	for segment := range strings.SplitSeq(path, "/") {
		if s := strings.TrimSpace(segment); s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

var queryEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func quote(s string) string {
	return "'" + queryEscaper.Replace(s) + "'"
}
