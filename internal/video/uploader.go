package video

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/ui"
	"tg-storage-assistant/internal/util"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func ProcessVideo(
	client *telegram.Client,
	ctx context.Context,
	peer tg.InputPeerClass,
	filePath, tag, description, tempDir string,
	maxSize int64,
) (int, []string, error) {
	logger.Info.Println("┏━━━━━━━━━━━━━━━ Processing video... ━━━━━━━━━━━━━━━┓")

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get file info: %w", err)
	}
	logger.Info.Printf("  FILE_NAME: %s", filePath)
	logger.Info.Printf("  TAG: %s", tag)
	logger.Info.Printf("  DESCRIPTION: %s", description)
	logger.Info.Printf("  SIZE: %s", util.FormatBytesToHumanReadable(fileInfo.Size()))

	// Step 1: Generate preview thumbnail (5×6 grid, 30 frames)
	logger.Info.Printf("Extracting 30 frames for preview...")
	frames, err := ExtractFrames(filePath, 30, tempDir)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to extract frames: %w", err)
	}

	previewPath := filepath.Join(tempDir, fmt.Sprintf("%s_%s_preview.jpg", tag, description))
	logger.Info.Printf("Composing preview grid...")
	if err := ComposeGrid(frames, 5, 6, previewPath); err != nil {
		return 0, nil, fmt.Errorf("failed to compose grid: %w", err)
	}

	// Step 2: Split video if needed
	logger.Info.Printf("Splitting video into parts if needed...")
	videoParts, err := splitVideo(filePath, maxSize, tempDir)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to split video: %w", err)
	}

	// Step 3: Validate media group size
	if 1+len(videoParts) > 10 {
		return 0, nil, fmt.Errorf("media group would have %d items (1 preview + %d video parts), exceeds Telegram limit of 10",
			1+len(videoParts), len(videoParts))
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

	logger.Info.Printf("Preparing album with %d items: 1 preview + %d video parts...", len(mediaItems), len(videoParts))

	for i, item := range mediaItems {
		fileInfo, err := os.Stat(item.FilePath)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get file info: %w", err)
		}
		logger.Debug.Printf("┃ #%d (%s - %-9s)[%s] %s\n",
			i+1,
			item.MediaType, util.FormatBytesToHumanReadable(fileInfo.Size()),
			util.SafeBase(item.FilePath), item.Caption)
	}

	up := uploader.NewUploader(client.API()).
		WithPartSize(512 * 1024).
		WithProgress(ui.NewUploadProgress())
	album := []tg.InputSingleMedia{}
	for _, item := range mediaItems {
		inputFile, err := up.FromPath(ctx, item.FilePath)
		if err != nil {
			return 0, nil, fmt.Errorf("upload %q: %w", item.FilePath, err)
		}
		logger.Debug.Println("uploaded item: ", inputFile)

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
		return 0, nil, err
	}

	logger.Info.Println("┗━━━━━━━━━━━ Video successfully uploaded ━━━━━━━━━━━┛")
	return 0, nil, nil
}

func LogFileInfo(filename string, size int64, success bool, err error) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	sizeKB := float64(size) / 1024.0
	if err != nil {
		logger.Warn.Printf("[%s] %s (%.2f KB) - Error: %v", status, filename, sizeKB, err)
	} else {
		logger.Info.Printf("[%s] %s (%.2f KB)", status, filename, sizeKB)
	}
}

func MoveVideoFiles(cfg *config.Config, originalFilename string, messageID int, additionalFiles []string) error {
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
		return nil, err
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
			return nil, err
		}
		result = append(result, outputPath)

		newDuration, err := getVideoDuration(outputPath)
		if err != nil {
			return nil, err
		}

		curDuration += newDuration
		i++
	}

	return result, nil
}
