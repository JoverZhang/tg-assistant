package telegram

import (
	"fmt"
	"path/filepath"
	"strings"

	tele "gopkg.in/telebot.v4"
)

// MediaItem represents a media item for group upload
type MediaItem struct {
	FilePath  string
	MediaType string // "photo" or "video"
	Caption   string
}

// Uploader handles Telegram file uploads
type Uploader struct {
	bot *tele.Bot
}

// NewUploader creates a new Telegram uploader (write-only, no polling)
func NewUploader(token string) (*Uploader, error) {
	bot, err := tele.NewBot(tele.Settings{
		Token: token,
		// No Poller - write-only client
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	return &Uploader{bot: bot}, nil
}

// SendMedia uploads a file to the specified chat with a caption
// Returns the message ID from the upload response
func (u *Uploader) SendMedia(chatID int64, filePath, caption string) (int, error) {
	recipient := &tele.Chat{ID: chatID}
	ext := strings.ToLower(filepath.Ext(filePath))
	file := tele.FromDisk(filePath)

	var msg *tele.Message
	var err error

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		msg, err = u.bot.Send(recipient, &tele.Photo{
			File:    file,
			Caption: caption,
		})
	case ".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv":
		msg, err = u.bot.Send(recipient, &tele.Video{
			File:    file,
			Caption: caption,
		})
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac":
		msg, err = u.bot.Send(recipient, &tele.Audio{
			File:    file,
			Caption: caption,
		})
	default:
		msg, err = u.bot.Send(recipient, &tele.Document{
			File:    file,
			Caption: caption,
		})
	}

	if err != nil {
		return 0, fmt.Errorf("failed to send media: %w", err)
	}

	if msg == nil {
		return 0, fmt.Errorf("received nil message response")
	}

	return msg.ID, nil
}

// SendMediaGroup uploads multiple media items as an album/media group
// Returns the message ID from the first message in the group
func (u *Uploader) SendMediaGroup(chatID int64, items []MediaItem) (int, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("no media items provided")
	}

	if len(items) > 10 {
		return 0, fmt.Errorf("too many media items: %d (Telegram limit is 10)", len(items))
	}

	recipient := &tele.Chat{ID: chatID}
	album := tele.Album{}

	for i, item := range items {
		file := tele.FromDisk(item.FilePath)

		switch item.MediaType {
		case "photo":
			album = append(album, &tele.Photo{
				File:    file,
				Caption: item.Caption,
			})
		case "video":
			album = append(album, &tele.Video{
				File:    file,
				Caption: item.Caption,
			})
		default:
			return 0, fmt.Errorf("unsupported media type for item %d: %s (use 'photo' or 'video')", i, item.MediaType)
		}
	}

	messages, err := u.bot.SendAlbum(recipient, album)
	if err != nil {
		return 0, fmt.Errorf("failed to send media group: %w", err)
	}

	if len(messages) == 0 {
		return 0, fmt.Errorf("received empty message response")
	}

	// Return the message ID from the first message in the group
	return messages[0].ID, nil
}
