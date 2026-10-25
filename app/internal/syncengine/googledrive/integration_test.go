package googledrive

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestIntegrationGoogleDrive(t *testing.T) {
	clientID, clientSecret, refreshToken := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"), os.Getenv("GOOGLE_REFRESH_TOKEN")
	folderID := os.Getenv("GOOGLE_DRIVE_FOLDER_ID")
	if clientID == "" || clientSecret == "" || refreshToken == "" || folderID == "" {
		t.Skip("set GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REFRESH_TOKEN and GOOGLE_DRIVE_FOLDER_ID to run against a real Drive folder")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cfg := OAuthConfig(clientID, clientSecret)
	engine, err := Open(ctx, cfg, refreshToken, folderID)
	if err != nil {
		t.Fatal(err)
	}

	name := "url2supernote integration " + time.Now().UTC().Format("20060102T150405Z") + ".epub"
	if err := engine.Put(ctx, name, []byte("integration test")); err != nil {
		t.Fatal(err)
	}
	exists, err := engine.Exists(ctx, name)
	if err != nil || !exists {
		t.Fatalf("Exists after Put = %v, %v", exists, err)
	}
	t.Logf("uploaded %q to folder %s; delete it manually", name, folderID)
}
