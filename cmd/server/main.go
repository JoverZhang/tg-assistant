package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	tele "gopkg.in/telebot.v4"
)

type MediaType string

const (
	MediaPhoto MediaType = "photo"
	MediaVideo MediaType = "video"
)

type MediaRecord struct {
	ChatID    int64
	MessageID int
	Type      MediaType
	FileID    string
	FileUID   string
	Caption   string
	UnixTime  int64
	FileName  string
	MimeType  string
	FileSize  int64
}

type MemStore struct {
	mu   sync.RWMutex
	data map[int64]map[int]*MediaRecord
}

func NewMemStore() *MemStore {
	return &MemStore{data: make(map[int64]map[int]*MediaRecord)}
}

func (s *MemStore) Put(r *MediaRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[r.ChatID]; !ok {
		s.data[r.ChatID] = make(map[int]*MediaRecord)
	}
	s.data[r.ChatID][r.MessageID] = r
}

func (s *MemStore) Get(chatID int64, msgID int) (*MediaRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.data[chatID]
	if !ok {
		return nil, false
	}
	r, ok := m[msgID]
	return r, ok
}

var store = NewMemStore()

func main() {
	_ = godotenv.Load()

	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("TOKEN is empty; set TOKEN in .env")
	}

	b, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	b.Handle("/hello", func(c tele.Context) error {
		return c.Send("Hello!")
	})

	// Handle incoming photos (v4: msg.Photo is *tele.Photo)
	b.Handle(tele.OnPhoto, func(c tele.Context) error {
		msg := c.Message()
		if msg.Photo == nil {
			return nil
		}
		p := msg.Photo
		rec := &MediaRecord{
			ChatID:    c.Chat().ID,
			MessageID: msg.ID,
			Type:      MediaPhoto,
			FileID:    p.FileID,
			FileUID:   p.UniqueID,
			Caption:   msg.Caption,
			UnixTime:  int64(msg.Unixtime),
			FileSize:  int64(p.FileSize),
		}
		store.Put(rec) // ✅ Fixed here
		return c.Reply(fmt.Sprintf("✅ Photo saved. message_id=%d", msg.ID))
	})

	// Handle incoming videos
	b.Handle(tele.OnVideo, func(c tele.Context) error {
		msg := c.Message()
		v := msg.Video
		if v == nil {
			return nil
		}
		rec := &MediaRecord{
			ChatID:    c.Chat().ID,
			MessageID: msg.ID,
			Type:      MediaVideo,
			FileID:    v.FileID,
			FileUID:   v.UniqueID,
			Caption:   msg.Caption,
			UnixTime:  int64(msg.Unixtime),
			FileName:  v.FileName,
			MimeType:  v.MIME,
			FileSize:  v.FileSize, // int64
		}
		store.Put(rec)
		return c.Reply(fmt.Sprintf("✅ Video saved. message_id=%d", msg.ID))
	})

	// Resend media as-is: /get <message_id>
	b.Handle("/get", func(c tele.Context) error {
		msgID, err := parseMsgIDArg(c)
		if err != nil {
			return c.Reply("Usage: /get <message_id>")
		}
		rec, ok := store.Get(c.Chat().ID, msgID)
		if !ok {
			return c.Reply("Message ID not found (currently in-memory only, please send a media first)")
		}
		switch rec.Type {
		case MediaPhoto:
			return c.Send(&tele.Photo{File: tele.File{FileID: rec.FileID}, Caption: rec.Caption})
		case MediaVideo:
			return c.Send(&tele.Video{File: tele.File{FileID: rec.FileID}, Caption: rec.Caption, MIME: rec.MimeType})
		default:
			return c.Reply("Unsupported media type")
		}
	})

	// Download to local: /dl <message_id>
	b.Handle("/dl", func(c tele.Context) error {
		msgID, err := parseMsgIDArg(c)
		if err != nil {
			return c.Reply("Usage: /dl <message_id>")
		}
		rec, ok := store.Get(c.Chat().ID, msgID)
		if !ok {
			return c.Reply("Message ID not found (currently in-memory only, please send a media first)")
		}
		path, err := downloadByRecord(b, rec)
		if err != nil {
			return c.Reply("Download failed: " + err.Error())
		}
		return c.Reply("Downloaded to local: " + path)
	})

	log.Println("Bot started...")
	b.Start()
}

func parseMsgIDArg(c tele.Context) (int, error) {
	arg := strings.TrimSpace(c.Message().Payload) // /get 123 -> "123"
	if arg == "" {
		return 0, errors.New("missing")
	}
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		return 0, errors.New("bad")
	}
	return id, nil
}

func downloadByRecord(b *tele.Bot, rec *MediaRecord) (string, error) {
	if err := os.MkdirAll("downloads", 0o755); err != nil {
		return "", err
	}
	file := tele.File{FileID: rec.FileID}

	ext := ".bin"
	switch rec.Type {
	case MediaPhoto:
		ext = ".jpg"
	case MediaVideo:
		ext = ".mp4"
	}
	name := rec.FileName
	if name == "" {
		name = fmt.Sprintf("%d_%d%s", rec.ChatID, rec.MessageID, ext)
	} else if filepath.Ext(name) == "" {
		name += ext
	}
	dst := filepath.Join("downloads", name)

	// ✅ Use Download directly, it will parse file_path internally and download
	if err := b.Download(&file, dst); err != nil {
		return "", err
	}
	return dst, nil
}
