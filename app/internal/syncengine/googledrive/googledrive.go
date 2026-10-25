// Package googledrive is the Google Drive Sync Engine.
package googledrive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

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

// The Google Picker for desktop apps is documented on the v2 authorization endpoint:
// https://developers.google.com/workspace/drive/picker/guides/desktop-mobile-picker
const pickerAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"

// ErrBroadScope means the refresh token grants more than the drive.file scope.
var ErrBroadScope = errors.New("the Google refresh token grants more than access to the Destination folder")

// OAuthConfig returns the OAuth client configuration for the Owner's own Google Cloud OAuth client. It asks
// only for the drive.file scope: the Owner picks the Destination folder in the Google Picker, and the app can
// then reach that folder and the files it creates, nothing else in the Drive.
func OAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: pickerAuthURL, TokenURL: google.Endpoint.TokenURL, AuthStyle: google.Endpoint.AuthStyle},
		Scopes:       []string{drive.DriveFileScope},
	}
}

var folderIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidFolderID reports whether id has the shape of a Drive folder ID.
func ValidFolderID(id string) bool {
	return folderIDPattern.MatchString(id)
}

// Engine stores files in a Destination folder on Google Drive.
type Engine struct {
	files    *drive.FilesService
	folderID string
	name     string
}

var _ syncengine.Engine = (*Engine)(nil)

// Open returns an Engine for the Destination folder with the given ID. It refreshes the token once, returns
// ErrBroadScope unless Google granted exactly the drive.file scope, and reads the folder's name.
func Open(ctx context.Context, cfg *oauth2.Config, refreshToken, folderID string) (*Engine, error) {
	longLived := context.WithoutCancel(ctx)
	tokens := oauth2.ReuseTokenSource(nil, cfg.TokenSource(longLived, &oauth2.Token{RefreshToken: refreshToken}))
	return open(ctx, oauth2.NewClient(longLived, tokens), tokens, folderID)
}

func open(ctx context.Context, client *http.Client, tokens oauth2.TokenSource, folderID string) (*Engine, error) {
	if !ValidFolderID(folderID) {
		return nil, fmt.Errorf("%q is not a Drive folder ID", folderID)
	}
	token, err := tokens.Token()
	if err != nil {
		return nil, classify(err)
	}
	scope, _ := token.Extra("scope").(string)
	if !slices.Equal(strings.Fields(scope), []string{drive.DriveFileScope}) {
		return nil, fmt.Errorf("%w (%s); run `url2supernote auth google` again", ErrBroadScope, scope)
	}

	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create drive client: %w", err)
	}
	folder, err := service.Files.Get(folderID).Fields("name", "mimeType").Context(ctx).Do()
	if err != nil {
		return nil, classify(err)
	}
	if folder.MimeType != folderMimeType {
		return nil, fmt.Errorf("%q is not a folder; run `url2supernote auth google` and pick the INBOX folder itself", folder.Name)
	}
	return &Engine{files: service.Files, folderID: folderID, name: folder.Name}, nil
}

// Name returns the Destination folder's name, e.g. "INBOX".
func (e *Engine) Name() string {
	return e.name
}

func (e *Engine) Exists(ctx context.Context, name string) (bool, error) {
	list, err := e.files.List().
		Q(fmt.Sprintf("name = %s and %s in parents and trashed = false", quote(name), quote(e.folderID))).
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
	_, err := e.files.Create(&drive.File{Name: name, Parents: []string{e.folderID}, MimeType: epubMimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(epubMimeType)).
		Fields("id").
		Context(ctx).
		Do()
	return classify(err)
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
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %w", syncengine.ErrUnauthorized, err)
		case http.StatusForbidden:
			if isLimitExceeded(apiErr) {
				return err
			}
			return fmt.Errorf("%w: %w", syncengine.ErrUnauthorized, err)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %w", syncengine.ErrDestinationNotFound, err)
		}
	}
	return err
}

func isLimitExceeded(err *googleapi.Error) bool {
	for _, item := range err.Errors {
		switch item.Reason {
		case "rateLimitExceeded", "userRateLimitExceeded", "dailyLimitExceeded", "storageQuotaExceeded":
			return true
		}
	}
	return false
}

var queryEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func quote(s string) string {
	return "'" + queryEscaper.Replace(s) + "'"
}
