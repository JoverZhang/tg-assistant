package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/dialer"
	"tg-storage-assistant/internal/fileprocessor"
	"tg-storage-assistant/internal/video"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func main() {
	ctx := context.Background()

	// Parse configuration from command-line arguments
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Check if ffmpeg and ffprobe are available (required for video processing)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("WARNING: ffmpeg not found in PATH. Video processing will fail.")
		log.Println("Please install ffmpeg to enable video processing features.")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		log.Println("WARNING: ffprobe not found in PATH. Video processing will fail.")
		log.Println("Please install ffprobe (usually bundled with ffmpeg).")
	}

	// Network settings
	dial, err := dialer.CreateProxyDialerFromURL(cfg.ProxyURL)
	if err != nil {
		log.Fatalf("Failed to create proxy dialer: %v", err)
	}

	// Session storage
	st := &telegram.FileSessionStorage{Path: "session.json"}

	// Client
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, telegram.Options{
		SessionStorage: st,
		Resolver: dcs.Plain(dcs.PlainOptions{
			Dial: dial.DialContext,
		}),
	})

	// Login flow
	flow := auth.NewFlow(
		auth.CodeOnly(cfg.Phone, &codeOnlyAuth{}),
		auth.SendCodeOptions{},
	)

	// Run client
	if err := client.Run(ctx, func(ctx context.Context) error {
		// Login if necessary
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("auth failed: %w", err)
		}

		// Scan for files
		processor := fileprocessor.NewProcessor(cfg.LocalDir, cfg.DoneDir)
		files, err := processor.ScanFiles()
		if err != nil {
			log.Fatalf("Failed to scan files: %v", err)
		}

		if len(files) == 0 {
			return fmt.Errorf("no files to process")
		}

		peer, err := resolvePeer(client.API(), ctx, cfg.StorageChatID)
		if err != nil {
			return fmt.Errorf("resolve peer: %w", err)
		}

		log.Printf("Found %d files to process", len(files))

		// Process each file
		stats := fileprocessor.Stats{}
		for _, filename := range files {
			stats.Processed++

			// Parse filename
			tag, description, err := fileprocessor.ParseFilename(filename)
			if err != nil {
				log.Printf("WARNING: Skipping file %s - %v", filename, err)
				stats.Failed++
				continue
			}

			// Get full file path
			filePath := processor.GetFilePath(filename)

			// Get file info for logging
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				log.Printf("WARNING: Failed to get file info for %s - %v", filename, err)
				stats.Failed++
				continue
			}

			if !fileprocessor.IsVideoFile(filename) {
				log.Printf("WARNING: Skipping non-video file: %s", filename)
				stats.Failed++
				continue
			}

			// Process video
			log.Printf("Processing video: %s", filename)
			msgID, additionalFiles, err := processVideo(client, ctx, peer, filePath, tag, description, cfg.MaxSize)
			if err != nil {
				logFileInfo(filename, fileInfo.Size(), false, err)
				stats.Failed++
				continue
			}

			// Move video and additional files to done directory
			if err := moveVideoFiles(cfg, filename, msgID, additionalFiles); err != nil {
				log.Printf("WARNING: Uploaded %s (msg ID: %d) but failed to move files - %v", filename, msgID, err)
				stats.Failed++
				continue
			}
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

func processVideo(
	client *telegram.Client,
	ctx context.Context,
	peer tg.InputPeerClass,
	filePath, tag, description string,
	maxSize int64,
) (int, []string, error) {
	// Create temporary directory for video processing
	tempDir := filepath.Join(os.TempDir(), "uploader_video_processing")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return 0, nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}
	defer cleanup()

	// Step 1: Generate preview thumbnail (5×6 grid, 30 frames)
	log.Printf("Extracting 30 frames for preview...")
	frames, err := video.ExtractFrames(filePath, 30, tempDir)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to extract frames: %w", err)
	}

	previewPath := filepath.Join(tempDir, fmt.Sprintf("%s_%s_preview.jpg", tag, description))
	log.Printf("Composing preview grid...")
	if err := video.ComposeGrid(frames, 5, 6, previewPath); err != nil {
		return 0, nil, fmt.Errorf("failed to compose grid: %w", err)
	}

	// Step 2: Split video if needed
	log.Printf("Checking video size for splitting...")
	videoParts, err := splitVideo(filePath, maxSize, tempDir)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to split video: %w", err)
	}

	// Step 3: Validate media group size
	if err := video.ValidateChunkCount(len(videoParts)); err != nil {
		return 0, nil, err
	}

	// Step 4: Build media group
	baseCaption := fmt.Sprintf("#%s %s", tag, strings.ReplaceAll(description, "_", " "))
	var mediaItems []MediaItem

	// First item: preview photo with caption (this is the only caption for the entire album)
	mediaItems = append(mediaItems, MediaItem{
		FilePath:  previewPath,
		MediaType: "photo",
		Caption:   baseCaption,
	})

	// Remaining items: video parts with empty captions
	// Telegram only shows the first item's caption for the entire album
	for _, partPath := range videoParts {
		mediaItems = append(mediaItems, MediaItem{
			FilePath:  partPath,
			MediaType: "video",
			Caption:   "",
		})
	}

	log.Printf("Uploading media group (%d items: 1 preview + %d video parts)...", len(mediaItems), len(videoParts))
	up := uploader.NewUploader(client.API()).WithPartSize(512 * 1024)
	album := []tg.InputSingleMedia{}
	for _, item := range mediaItems {
		log.Println("uploading item: ", item.FilePath)
		inputFile, err := up.FromPath(ctx, item.FilePath)
		if err != nil {
			return 0, nil, fmt.Errorf("upload %q: %w", item.FilePath, err)
		}
		log.Println("uploaded item: ", inputFile)

		switch item.MediaType {
		case "photo":
			album = append(album, buildPhotoMedia(client.API(), ctx, inputFile, item.Caption))
		case "video":
			album = append(album, buildVideoMedia(client.API(), ctx, inputFile, item.Caption))
		}
	}

	_, err = client.API().MessagesSendMultiMedia(ctx, &tg.MessagesSendMultiMediaRequest{
		Peer:       peer,
		MultiMedia: album,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("send album: %w", err)
	}
	log.Println("Album sent.")
	return 0, nil, nil
}

func logFileInfo(filename string, size int64, success bool, err error) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	sizeKB := float64(size) / 1024.0
	if err != nil {
		log.Printf("[%s] %s (%.2f KB) - Error: %v", status, filename, sizeKB, err)
	} else {
		log.Printf("[%s] %s (%.2f KB)", status, filename, sizeKB)
	}
}

func moveVideoFiles(cfg *config.Config, originalFilename string, messageID int, additionalFiles []string) error {
	// Move original video
	sourcePath := filepath.Join(cfg.LocalDir, originalFilename)
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	// Original video: originalname_msgid_<ID>.ext
	newFilename := fmt.Sprintf("%s_msgid_%d%s", nameWithoutExt, messageID, ext)
	destPath := filepath.Join(cfg.DoneDir, newFilename)

	if err := move(sourcePath, destPath); err != nil {
		return fmt.Errorf("failed to move original video: %w", err)
	}

	// Move additional files (preview, split parts)
	for _, addFile := range additionalFiles {
		addBasename := filepath.Base(addFile)
		addExt := filepath.Ext(addFile)
		addNameWithoutExt := strings.TrimSuffix(addBasename, addExt)

		// Add message ID to filename
		newAddFilename := fmt.Sprintf("%s_msgid_%d%s", addNameWithoutExt, messageID, addExt)
		addDestPath := filepath.Join(cfg.DoneDir, newAddFilename)

		if err := move(addFile, addDestPath); err != nil {
			return fmt.Errorf("failed to move additional file: %w", err)
		}
	}

	return nil
}

func move(src, dst string) error {
	return os.Rename(src, dst)
}

type MediaItem struct {
	FilePath  string
	MediaType string // "photo" or "video"
	Caption   string
}

func buildPhotoMedia(api *tg.Client, ctx context.Context, input tg.InputFileClass, caption string) tg.InputSingleMedia {
	media, err := api.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
		Peer:  &tg.InputPeerSelf{},
		Media: &tg.InputMediaUploadedPhoto{File: input},
	})
	if err != nil {
		panic(err)
	}
	return tg.InputSingleMedia{
		// Media:    &tg.InputMediaUploadedPhoto{File: input},
		Media: &tg.InputMediaPhoto{ID: &tg.InputPhoto{
			ID:            media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo).GetID(),
			AccessHash:    media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo).GetAccessHash(),
			FileReference: media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo).GetFileReference(),
		}},
		RandomID: randID(),
		Message:  caption,
	}
}
func buildVideoMedia(api *tg.Client, ctx context.Context, inputFile tg.InputFileClass, caption string) tg.InputSingleMedia {
	fileName := func() string {
		switch v := inputFile.(type) {
		case *tg.InputFile:
			return filepath.Base(v.Name)
		case *tg.InputFileBig:
			return filepath.Base(v.Name)
		default:
			return "Unknown"
		}
	}()

	attrs := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeVideo{SupportsStreaming: true},
		&tg.DocumentAttributeFilename{FileName: fileName},
	}
	media, err := api.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
		Peer: &tg.InputPeerSelf{},
		Media: &tg.InputMediaUploadedDocument{
			File:       inputFile,
			MimeType:   guessMIME(fileName),
			Attributes: attrs,
		},
	})
	if err != nil {
		panic(err)
	}
	return tg.InputSingleMedia{
		Media: &tg.InputMediaDocument{
			ID: &tg.InputDocument{
				ID:            media.(*tg.MessageMediaDocument).Document.(*tg.Document).GetID(),
				AccessHash:    media.(*tg.MessageMediaDocument).Document.(*tg.Document).GetAccessHash(),
				FileReference: media.(*tg.MessageMediaDocument).Document.(*tg.Document).GetFileReference(),
			},
		},
		RandomID: randID(),
		Message:  caption,
	}
}

func randID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]))
}

func guessMIME(path string) string {
	if mt := mime.TypeByExtension(filepath.Ext(path)); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

func resolvePeer(api *tg.Client, ctx context.Context, chatID int64) (tg.InputPeerClass, error) {
	// Get dialogs to find the peer with access hash
	dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dialogs: %w", err)
	}

	var chats []tg.ChatClass
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}

	// Find the chat
	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Channel:
			// Check if this is our target channel
			// Channel IDs in Bot API format: -100 + channel_id
			fullID := int64(-1000000000000) - c.ID
			if fullID == chatID {
				return &tg.InputPeerChannel{
					ChannelID:  c.ID,
					AccessHash: c.AccessHash,
				}, nil
			}
		case *tg.Chat:
			// Regular group chat
			if -int64(c.ID) == chatID {
				return &tg.InputPeerChat{
					ChatID: c.ID,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("chat ID %d not found in dialogs (make sure the user account is a member of this chat)", chatID)
}

type codeOnlyAuth struct{}

func (a *codeOnlyAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter authentication code: ")
	var code string
	fmt.Scanln(&code)
	return code, nil
}

func splitVideo(videoPath string, maxSize int64, outputDir string) ([]string, error) {
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	// If no maxSize specified or file is smaller, return original
	if maxSize <= 0 || fileSize <= maxSize {
		return []string{videoPath}, nil
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare output pattern
	ext := filepath.Ext(videoPath)
	baseName := filepath.Base(videoPath)
	baseName = baseName[:len(baseName)-len(ext)]
	outputPattern := filepath.Join(outputDir, fmt.Sprintf("%s_part%%03d%s", baseName, ext))

	totalDuration, err := getVideoDuration(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get video duration: %w", err)
	}

	// Split videos by specified maxSize
	result := []string{}
	curDuration := 0.0
	i := 0
	for curDuration < totalDuration {
		// Split video by maxSize
		outputPath := fmt.Sprintf(outputPattern, i)
		err := splitVideoByDuration(videoPath, outputPath, int64(curDuration), maxSize)
		if err != nil {
			return nil, fmt.Errorf("failed to split video: %w", err)
		}
		result = append(result, outputPath)

		newDuration, err := getVideoDuration(outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get video duration: %w", err)
		}

		curDuration += newDuration
		i++
	}

	return result, nil
}

func splitVideoByDuration(videoPath, outputPath string, beginDuration, maxSize int64) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-ss", strconv.FormatInt(beginDuration, 10),
		"-fs", strconv.FormatInt(maxSize, 10),
		"-c", "copy", // Copy codec (no re-encoding)
		"-y", // Overwrite output files
		outputPath)
	log.Println("Command: ", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to split video: %w", err)
	}
	return nil
}

func getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-i", videoPath,
		"-show_entries", "format=duration",
		"-v", "quiet",
		"-of", "default=noprint_wrappers=1:nokey=1")
	log.Println("Command: ", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}
	return duration, nil

}
