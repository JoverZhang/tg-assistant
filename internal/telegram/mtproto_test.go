package telegram

import (
	"os"
	"strconv"
	"testing"
)

// getTestConfig reads configuration from .env file via environment variables
func getTestConfig(t *testing.T) (apiID int, apiHash, sessionFile string, storageChatID int64, proxyURL string) {
	apiIDStr := os.Getenv("API_ID")
	apiHash = os.Getenv("API_HASH")
	sessionFile = os.Getenv("SESSION_FILE")
	if sessionFile == "" {
		sessionFile = "./session.json"
	}
	chatIDStr := os.Getenv("CHAT_ID")
	proxyURL = os.Getenv("PROXY_URL")

	if apiIDStr == "" || apiHash == "" || chatIDStr == "" {
		t.Fatal("Missing environment variables: API_ID, API_HASH, CHAT_ID")
	}

	var err error
	apiID, err = strconv.Atoi(apiIDStr)
	if err != nil {
		t.Fatalf("Invalid API_ID: %v", err)
	}

	storageChatID, err = strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		t.Fatalf("Invalid CHAT_ID: %v", err)
	}

	return
}

// TestUpload runs comprehensive upload tests
func TestUpload(t *testing.T) {
	apiID, apiHash, sessionFile, storageChatID, proxyURL := getTestConfig(t)

	// Create MTProto client once for all tests
	client, err := NewMTProtoClient(MTProtoConfig{
		SessionFile: sessionFile,
		APIID:       apiID,
		APIHash:     apiHash,
		ProxyURL:    proxyURL,
	})
	if err != nil {
		t.Fatalf("Failed to create MTProto client: %v", err)
	}
	defer client.Close()

	// Test 1: Upload Photo
	t.Run("UploadPhoto", func(t *testing.T) {
		photoPath := "/tmp/test-uploader/upload/test_photo.jpg"
		if _, err := os.Stat(photoPath); os.IsNotExist(err) {
			t.Fatalf("Test photo not found: %s (run 'make init-test-uploader' first)", photoPath)
		}

		caption := "#test Photo upload test"
		t.Logf("Uploading photo: %s", photoPath)
		msgID, err := client.SendMedia(storageChatID, photoPath, caption)
		if err != nil {
			t.Fatalf("Failed to upload photo: %v", err)
		}
		t.Logf("✓ Successfully uploaded photo, message ID: %d", msgID)
	})

	// Test 2: Upload Video
	t.Run("UploadVideo", func(t *testing.T) {
		videoPath := "/tmp/test-uploader/local/test_11mb.mp4"
		if _, err := os.Stat(videoPath); os.IsNotExist(err) {
			t.Fatalf("Test video not found: %s (run 'make init-test-uploader' first)", videoPath)
		}

		caption := "#test Video upload test"
		t.Logf("Uploading video: %s", videoPath)
		msgID, err := client.SendMedia(storageChatID, videoPath, caption)
		if err != nil {
			t.Fatalf("Failed to upload video: %v", err)
		}
		t.Logf("✓ Successfully uploaded video, message ID: %d", msgID)
	})

	// Test 3: Upload Media Group (Photo + 2 Videos in one message)
	t.Run("UploadMediaGroup", func(t *testing.T) {
		// Check all test files exist
		previewPath := "/tmp/test-uploader/upload/test_preview.jpg"
		videoPart1 := "/tmp/test-uploader/upload/test_part1.mp4"
		videoPart2 := "/tmp/test-uploader/upload/test_part2.mp4"

		for _, path := range []string{previewPath, videoPart1, videoPart2} {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("Test file not found: %s (run 'make init-test-uploader' first)", path)
			}
		}

		mediaItems := []MediaItem{
			{
				FilePath:  previewPath,
				MediaType: "photo",
				Caption:   "#test Media group upload test (1 photo + 2 videos)",
			},
			{
				FilePath:  videoPart1,
				MediaType: "video",
				Caption:   "",
			},
			{
				FilePath:  videoPart2,
				MediaType: "video",
				Caption:   "",
			},
		}

		t.Logf("Uploading media group: 1 photo + 2 videos")
		msgID, err := client.SendMediaGroup(storageChatID, mediaItems)
		if err != nil {
			t.Fatalf("Failed to upload media group: %v", err)
		}
		t.Logf("✓ Successfully uploaded media group, message ID: %d", msgID)
	})
}
