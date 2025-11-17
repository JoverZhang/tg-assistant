package client

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"tg-storage-assistant/internal/ffmpeg"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/util"

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

	album := []tg.InputSingleMedia{}
	for _, item := range items {
		media, err := c.uploadMedia(item)
		if err != nil {
			return err
		}
		album = append(album, *media)
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

func (c *Client) uploadMedia(media MediaItem) (*tg.InputSingleMedia, error) {
	inputFile, err := c.uploader.FromPath(c.ctx, media.FilePath)
	if err != nil {
		return nil, fmt.Errorf("upload %q: %w", media.FilePath, err)
	}
	logger.Debug.Println("uploaded media: ", inputFile)

	switch media.MediaType {
	case "photo":
		photo := c.buildPhotoMedia(inputFile, media.Caption)
		return &photo, nil
	case "video":
		width, height, err := ffmpeg.GetVideoResolution(media.FilePath)
		if err != nil {
			return nil, err
		}
		video := c.buildVideoMedia(inputFile, width, height, media.Caption)
		return &video, nil
	}

	return nil, fmt.Errorf("invalid media type: %s", media.MediaType)
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

func (c *Client) buildVideoMedia(inputFile tg.InputFileClass, width, height int, caption string) tg.InputSingleMedia {
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
		&tg.DocumentAttributeVideo{
			SupportsStreaming: true,
			W:                 width,
			H:                 height,
		},
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
