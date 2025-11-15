package client

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/ui"
	"tg-storage-assistant/internal/util"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

type MediaItem struct {
	FilePath  string
	MediaType string // "photo" or "video"
	Caption   string
}

func (c *Client) SendMultiMedia(peer tg.InputPeerClass, items []MediaItem) error {
	for i, item := range items {
		fileInfo, err := os.Stat(item.FilePath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %w", err)
		}
		logger.Debug.Printf("┃ #%d (%s - %-9s)[%s] %s\n",
			i+1,
			item.MediaType, util.FormatBytesToHumanReadable(fileInfo.Size()),
			util.SafeBase(item.FilePath), item.Caption)
	}

	up := uploader.NewUploader(c.client.API()).
		WithPartSize(512 * 1024).
		WithProgress(ui.NewUploadProgress())
	album := []tg.InputSingleMedia{}
	for _, item := range items {
		inputFile, err := up.FromPath(c.ctx, item.FilePath)
		if err != nil {
			return fmt.Errorf("upload %q: %w", item.FilePath, err)
		}
		logger.Debug.Println("uploaded item: ", inputFile)

		switch item.MediaType {
		case "photo":
			album = append(album, c.buildPhotoMedia(inputFile, item.Caption))
		case "video":
			album = append(album, c.buildVideoMedia(inputFile, item.Caption))
		}
	}

	_, err := c.client.API().MessagesSendMultiMedia(c.ctx, &tg.MessagesSendMultiMediaRequest{
		Peer:       peer,
		MultiMedia: album,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) buildPhotoMedia(input tg.InputFileClass, caption string) tg.InputSingleMedia {
	media, err := c.client.API().MessagesUploadMedia(c.ctx, &tg.MessagesUploadMediaRequest{
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

func (c *Client) buildVideoMedia(inputFile tg.InputFileClass, caption string) tg.InputSingleMedia {
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
	media, err := c.client.API().MessagesUploadMedia(c.ctx, &tg.MessagesUploadMediaRequest{
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
