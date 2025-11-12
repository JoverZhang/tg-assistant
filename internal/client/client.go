package client

import (
	"context"
	"fmt"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/dialer"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
)

type Client struct {
	ctx    context.Context
	cfg    *config.Config
	Client *telegram.Client
	flow   auth.Flow
}

func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	// Telegram options
	options := telegram.Options{}

	// Session settings
	options.SessionStorage = &telegram.FileSessionStorage{
		Path: cfg.SessionFile,
	}

	// Network settings
	if cfg.ProxyURL != "" {
		dial, err := dialer.CreateProxyDialerFromURL(cfg.ProxyURL)
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
		Client: client,
		flow:   flow,
	}, nil
}

func (c *Client) ResolvePeer(chatID int64) (tg.InputPeerClass, error) {
	// Get dialogs to find the peer with access hash
	dialogs, err := c.Client.API().MessagesGetDialogs(c.ctx, &tg.MessagesGetDialogsRequest{
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

func (c *Client) Run(f func(ctx context.Context) error) error {
	return c.Client.Run(c.ctx, func(ctx context.Context) error {
		if err := c.LoginIfNecessary(); err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		return f(c.ctx)
	})
}

func (c *Client) LoginIfNecessary() error {
	// Login if necessary
	if err := c.Client.Auth().IfNecessary(c.ctx, c.flow); err != nil {
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
