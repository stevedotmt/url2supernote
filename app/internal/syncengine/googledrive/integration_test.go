package googledrive

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestIntegrationGoogleDrive(t *testing.T) {
	clientID, clientSecret, refreshToken := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"), os.Getenv("GOOGLE_REFRESH_TOKEN")
	destination := os.Getenv("URL2SUPERNOTE_INTEGRATION_DESTINATION")
	if clientID == "" || clientSecret == "" || refreshToken == "" || destination == "" {
		t.Skip("set GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REFRESH_TOKEN and URL2SUPERNOTE_INTEGRATION_DESTINATION to run against a real Drive folder")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	engine, err := New(ctx, OAuthConfig(clientID, clientSecret), refreshToken, destination)
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
	t.Logf("uploaded %q to %s; delete it manually", name, destination)
}
