package telegram

import (
	"fmt"
	"path/filepath"
	"strings"

	tele "gopkg.in/telebot.v4"
)

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
