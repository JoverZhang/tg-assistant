package client

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/dialer"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/ui"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

type Client struct {
	ctx            context.Context
	cfg            *config.MtprotoConfig
	client         *telegram.Client
	flow           auth.Flow
	uploader       *uploader.Uploader
	uploadProgress *ui.UploadProgress
}

func NewClient(ctx context.Context, cfg *config.MtprotoConfig) (*Client, error) {
	// Telegram options
	options := telegram.Options{}

	// Session settings
	options.SessionStorage = &telegram.FileSessionStorage{
		Path: cfg.SessionFile,
	}

	// Network settings
	if cfg.Proxy != "" {
		dial, err := dialer.CreateProxyDialerFromURL(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
		}

		options.Resolver = dcs.Plain(dcs.PlainOptions{
			Dial: dial.DialContext,
		})
	}

	// Client
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, options)
	// Login flow
	flow := auth.NewFlow(
		auth.CodeOnly(cfg.Phone, &codeOnlyAuth{}),
		auth.SendCodeOptions{},
	)

	return &Client{
		ctx:    ctx,
		cfg:    cfg,
		client: client,
		flow:   flow,
	}, nil
}

func (c *Client) InitUploader() {
	c.uploadProgress = ui.NewUploadProgress()
	c.uploader = uploader.NewUploader(c.client.API()).
		WithPartSize(512 * 1024).
		WithProgress(c.uploadProgress)
}

func (c *Client) CloseUploader() {
	c.uploadProgress.Shutdown()
	c.uploader = nil
}

func (c *Client) ResolvePeer(chatID int64) (tg.InputPeerClass, error) {
	// Get dialogs to find the peer with access hash
	dialogs, err := c.client.API().MessagesGetDialogs(c.ctx, &tg.MessagesGetDialogsRequest{
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
		switch ch := chat.(type) {
		case *tg.Channel:
			// Check if this is our target channel
			// Channel IDs in Bot API format: -100 + channel_id
			fullID := int64(-1000000000000) - ch.ID
			if fullID == chatID {
				return &tg.InputPeerChannel{
					ChannelID:  ch.ID,
					AccessHash: ch.AccessHash,
				}, nil
			}
		case *tg.Chat:
			// Regular group chat
			if -int64(ch.ID) == chatID {
				fmt.Println("found chat: ", ch.Title)
				return &tg.InputPeerChat{
					ChatID: ch.ID,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("chat ID %d not found in dialogs (make sure the user account is a member of this chat)", chatID)
}

func (c *Client) Run(f func(ctx context.Context) error) error {
	return c.client.Run(c.ctx, func(ctx context.Context) error {
		if err := c.LoginIfNecessary(); err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		return f(c.ctx)
	})
}

func (c *Client) LoginIfNecessary() error {
	// Login if necessary
	if err := c.client.Auth().IfNecessary(c.ctx, c.flow); err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}
	return nil
}

type codeOnlyAuth struct{}

func (a *codeOnlyAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter authentication code: ")
	var code string
	fmt.Scanln(&code)
	return code, nil
}

// HistoryOptions is the options for GetHistory
type HistoryOptions struct {
	// Telegram rules: OffsetID=0 means from the latest.
	OffsetID int
	MinID    int
	MaxID    int
	Limit    int
}

func (c *Client) GetHistory(chatID int64, opts HistoryOptions) ([]*tg.Message, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	peer, err := c.ResolvePeer(chatID)
	if err != nil {
		return nil, fmt.Errorf("ResolvePeer failed: %w", err)
	}

	resp, err := c.client.API().MessagesGetHistory(c.ctx, &tg.MessagesGetHistoryRequest{
		Peer:       peer,
		OffsetID:   opts.OffsetID,
		AddOffset:  0,
		MinID:      opts.MinID,
		MaxID:      opts.MaxID,
		Limit:      opts.Limit,
		OffsetDate: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("MessagesGetHistory failed: %w", err)
	}

	var msgs []*tg.Message

	switch v := resp.(type) {
	case *tg.MessagesMessages:
		for _, m := range v.Messages {
			if msg, ok := m.(*tg.Message); ok {
				msgs = append(msgs, msg)
			}
		}
	case *tg.MessagesMessagesSlice:
		for _, m := range v.Messages {
			if msg, ok := m.(*tg.Message); ok {
				msgs = append(msgs, msg)
			}
		}
	case *tg.MessagesChannelMessages:
		for _, m := range v.Messages {
			if msg, ok := m.(*tg.Message); ok {
				msgs = append(msgs, msg)
			}
		}
	default:
		return nil, fmt.Errorf("unexpected history type %T", resp)
	}

	return msgs, nil
}

func (c *Client) ForwardMessages(fromChatID, toChatID int64, msgs []*tg.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	fromPeer, err := c.ResolvePeer(fromChatID)
	if err != nil {
		return fmt.Errorf("ResolvePeer(from) failed: %w", err)
	}
	toPeer, err := c.ResolvePeer(toChatID)
	if err != nil {
		return fmt.Errorf("ResolvePeer(to) failed: %w", err)
	}

	// Order by ID from old to new
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].ID < msgs[j].ID
	})

	ids := make([]int, len(msgs))
	randomIDs := make([]int64, len(msgs))

	// Generate IDs and random IDs
	for i, m := range msgs {
		ids[i] = m.ID
		randomIDs[i] = randID()
	}

	_, err = c.client.API().MessagesForwardMessages(c.ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer,
		ID:       ids,
		RandomID: randomIDs,
		ToPeer:   toPeer,
	})
	if err != nil {
		return fmt.Errorf("MessagesForwardMessages failed: %w", err)
	}

	return nil
}

func (c *Client) SendMessagesAsNew(fromChatID, toChatID int64, msgs []*tg.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	toPeer, err := c.ResolvePeer(toChatID)
	if err != nil {
		return fmt.Errorf("ResolvePeer(to) failed: %w", err)
	}

	// Order by ID from old to new
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].ID < msgs[j].ID
	})

	api := c.client.API()

	// 1. Split into singles and albums
	singles := make([]*tg.Message, 0, len(msgs))
	albums := make(map[int64][]*tg.Message) // groupedID -> msgs

	for _, m := range msgs {
		if m.GroupedID == 0 {
			singles = append(singles, m)
			continue
		}
		gid := m.GroupedID
		albums[gid] = append(albums[gid], m)
	}

	// 2. Send singles first
	for _, m := range singles {
		// Plain text
		if m.Media == nil {
			if strings.TrimSpace(m.Message) == "" {
				continue
			}
			_, err := api.MessagesSendMessage(c.ctx, &tg.MessagesSendMessageRequest{
				Peer:     toPeer,
				RandomID: randID(),
				Message:  m.Message,
			})
			if err != nil {
				return fmt.Errorf("sendMessage id=%d failed: %w", m.ID, err)
			}
			continue
		}

		switch media := m.Media.(type) {
		case *tg.MessageMediaPhoto:
			photo, ok := media.Photo.(*tg.Photo)
			if !ok || photo == nil {
				continue
			}
			_, err := api.MessagesSendMedia(c.ctx, &tg.MessagesSendMediaRequest{
				Peer:     toPeer,
				RandomID: randID(),
				Media: &tg.InputMediaPhoto{
					ID: &tg.InputPhoto{
						ID:            photo.ID,
						AccessHash:    photo.AccessHash,
						FileReference: photo.FileReference,
					},
				},
				Message: m.Message, // caption
			})
			if err != nil {
				return fmt.Errorf("sendMedia(photo) id=%d failed: %w", m.ID, err)
			}

		case *tg.MessageMediaDocument:
			doc, ok := media.Document.(*tg.Document)
			if !ok || doc == nil {
				continue
			}
			_, err := api.MessagesSendMedia(c.ctx, &tg.MessagesSendMediaRequest{
				Peer:     toPeer,
				RandomID: randID(),
				Media: &tg.InputMediaDocument{
					ID: &tg.InputDocument{
						ID:            doc.ID,
						AccessHash:    doc.AccessHash,
						FileReference: doc.FileReference,
					},
				},
				Message: m.Message, // caption
			})
			if err != nil {
				return fmt.Errorf("sendMedia(document) id=%d failed: %w", m.ID, err)
			}
		default:
			// Ignore other types
			logger.Debug.Printf("unknown media type: %T\n", m.Media)
			continue
		}
	}

	// 3. Send albums: group by GroupedID using sendMultiMedia
	for gid, group := range albums {
		// Order by ID in the group to ensure consistency
		sort.Slice(group, func(i, j int) bool {
			return group[i].ID < group[j].ID
		})

		var multi []tg.InputSingleMedia
		for i, m := range group {
			if m.Media == nil {
				// Plain text in albums is usually not present, ignore
				logger.Debug.Printf("plain text in album id=%d\n", m.ID)
				continue
			}

			var mediaInput tg.InputMediaClass

			switch media := m.Media.(type) {
			case *tg.MessageMediaPhoto:
				photo, ok := media.Photo.(*tg.Photo)
				if !ok || photo == nil {
					continue
				}
				mediaInput = &tg.InputMediaPhoto{
					ID: &tg.InputPhoto{
						ID:            photo.ID,
						AccessHash:    photo.AccessHash,
						FileReference: photo.FileReference,
					},
				}

			case *tg.MessageMediaDocument:
				doc, ok := media.Document.(*tg.Document)
				if !ok || doc == nil {
					continue
				}
				mediaInput = &tg.InputMediaDocument{
					ID: &tg.InputDocument{
						ID:            doc.ID,
						AccessHash:    doc.AccessHash,
						FileReference: doc.FileReference,
					},
				}

			default:
				// Unsupported media types are skipped
				logger.Debug.Printf("unsupported media type: %T\n", m.Media)
				continue
			}

			// Only include caption on the first message in the album (consistent with telebot behavior)
			caption := ""
			if i == 0 {
				caption = m.Message
			}

			multi = append(multi, tg.InputSingleMedia{
				Media:    mediaInput,
				RandomID: randID(),
				Message:  caption,
			})
		}

		if len(multi) == 0 {
			continue
		}

		_, err := api.MessagesSendMultiMedia(c.ctx, &tg.MessagesSendMultiMediaRequest{
			Peer:       toPeer,
			MultiMedia: multi,
		})
		if err != nil {
			return fmt.Errorf("sendMultiMedia(grouped_id=%d) failed: %w", gid, err)
		}
	}

	return nil
}
