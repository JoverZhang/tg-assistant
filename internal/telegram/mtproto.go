package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
)

// MTProtoClient handles MTProto-based Telegram file uploads
type MTProtoClient struct {
	client *telegram.Client
	api    *tg.Client
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{}
}

// MTProtoConfig holds configuration for MTProto client
type MTProtoConfig struct {
	SessionFile string
	APIID       int
	APIHash     string
	Phone       string
	ProxyURL    string // Optional: e.g., "socks5://127.0.0.1:1080" or "http://127.0.0.1:8080"
}

// NewMTProtoClient creates a new MTProto client with session management
func NewMTProtoClient(cfg MTProtoConfig) (*MTProtoClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create session storage
	sessionStorage := &telegram.FileSessionStorage{
		Path: cfg.SessionFile,
	}

	// Build client options
	options := telegram.Options{
		SessionStorage: sessionStorage,
		Logger:         zap.L(),
	}

	// Configure proxy if provided
	if cfg.ProxyURL != "" {
		dialer, err := createProxyDialer(cfg.ProxyURL)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
		}
		options.Resolver = dcs.Plain(dcs.PlainOptions{
			Dial: dialer.DialContext,
		})
		fmt.Printf("✓ Using proxy: %s\n", cfg.ProxyURL)
	}

	// Create client
	client := telegram.NewClient(cfg.APIID, cfg.APIHash, options)

	mtpClient := &MTProtoClient{
		client: client,
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
	}

	// Run client in background
	errChan := make(chan error, 1)
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Store API client
			mtpClient.api = client.API()

			// Check authorization status
			status, err := client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get auth status: %w", err)
			}

			if !status.Authorized {
				// Need to authenticate
				if cfg.Phone == "" {
					return fmt.Errorf("phone number required for authentication")
				}

				fmt.Println("Authenticating...")

				// Use terminal auth flow
				flow := auth.NewFlow(
					&terminalAuth{phone: cfg.Phone},
					auth.SendCodeOptions{},
				)

				if err := client.Auth().IfNecessary(ctx, flow); err != nil {
					return fmt.Errorf("authentication failed: %w", err)
				}

				fmt.Println("✓ Authentication successful, session saved to", cfg.SessionFile)
			} else {
				fmt.Println("✓ Using existing session from", cfg.SessionFile)
			}

			// Signal that client is ready
			close(mtpClient.ready)

			// Keep client running
			<-ctx.Done()
			return ctx.Err()
		})

		errChan <- err
	}()

	// Wait for client to be ready or error
	select {
	case <-mtpClient.ready:
		return mtpClient, nil
	case err := <-errChan:
		cancel()
		if err != nil && err != context.Canceled {
			return nil, fmt.Errorf("failed to initialize client: %w", err)
		}
		return nil, fmt.Errorf("client initialization failed")
	}
}

// terminalAuth implements auth flow for terminal input
type terminalAuth struct {
	phone string
}

func (a *terminalAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a *terminalAuth) Password(_ context.Context) (string, error) {
	fmt.Print("Enter 2FA password: ")
	var password string
	fmt.Scanln(&password)
	return password, nil
}

func (a *terminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter authentication code: ")
	var code string
	fmt.Scanln(&code)
	return code, nil
}

func (a *terminalAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a *terminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported")
}

// SendMedia uploads a single file to the specified chat with a caption
// Returns the message ID on success
func (c *MTProtoClient) SendMedia(chatID int64, filePath, caption string) (int, error) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Upload file
	u := uploader.NewUploader(c.api)
	upload, err := u.FromPath(c.ctx, filePath)
	if err != nil {
		return 0, fmt.Errorf("upload %q: %w", filePath, err)
	}

	// Determine media type and send
	ext := strings.ToLower(filepath.Ext(filePath))
	var inputMedia tg.InputMediaClass

	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" || ext == ".bmp" {
		// For photos, use InputMediaUploadedPhoto to show preview
		inputMedia = &tg.InputMediaUploadedPhoto{
			File: upload,
		}
	} else {
		// Video, audio, documents all use InputMediaUploadedDocument
		attributes := []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{
				FileName: filepath.Base(filePath),
			},
		}

		// Add video attribute for video files
		if isVideoExtension(ext) {
			attributes = append(attributes, &tg.DocumentAttributeVideo{
				Duration: 0, // Unknown duration is fine
				W:        0, // Unknown width/height is fine
				H:        0,
			})
		}

		inputMedia = &tg.InputMediaUploadedDocument{
			File:       upload,
			MimeType:   getMimeType(ext),
			Attributes: attributes,
		}
	}

	// Convert chatID to proper peer
	peer, err := c.resolvePeer(chatID)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve peer: %w", err)
	}

	// Send message
	updates, err := c.api.MessagesSendMedia(c.ctx, &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    inputMedia,
		Message:  caption,
		RandomID: generateRandomID(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to send media: %w", err)
	}

	// Extract message ID from updates
	msgID := extractMessageID(updates)
	if msgID == 0 {
		return 0, fmt.Errorf("failed to get message ID from response")
	}

	return msgID, nil
}

// SendMediaGroup uploads multiple media items as an album/media group
// Returns the base message ID from the first message in the group
func (c *MTProtoClient) SendMediaGroup(chatID int64, items []MediaItem) (int, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("no media items provided")
	}

	if len(items) > 10 {
		return 0, fmt.Errorf("too many media items: %d (Telegram limit is 10)", len(items))
	}

	peer, err := c.resolvePeer(chatID)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve peer: %w", err)
	}

	up := uploader.NewUploader(c.api).WithPartSize(512 * 1024)

	multiMedia := []tg.InputSingleMedia{}
	for _, item := range items {
		inputFile, err := up.FromPath(c.ctx, item.FilePath)
		if err != nil {
			return 0, fmt.Errorf("upload photo1: %w", err)
		}

		switch item.MediaType {
		case "photo":
			multiMedia = append(multiMedia, c.buildPhotoMedia(inputFile, item.Caption))
		case "video":
			multiMedia = append(multiMedia, c.buildVideoMedia(inputFile, item.Caption))
		default:
			return 0, fmt.Errorf("unsupported media type: %s", item.MediaType)
		}
	}

	// Send multi-media
	updates, err := c.api.MessagesSendMultiMedia(c.ctx, &tg.MessagesSendMultiMediaRequest{
		Peer:       peer,
		MultiMedia: multiMedia,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to send media group: %w", err)
	}

	// Extract message ID from first message
	msgID := extractMessageID(updates)
	if msgID == 0 {
		return 0, fmt.Errorf("failed to get message ID from response")
	}

	return msgID, nil
}

func (c *MTProtoClient) buildPhotoMedia(inputFile tg.InputFileClass, caption string) tg.InputSingleMedia {
	media, err := c.api.MessagesUploadMedia(c.ctx, &tg.MessagesUploadMediaRequest{
		Peer:  &tg.InputPeerSelf{},
		Media: &tg.InputMediaUploadedPhoto{File: inputFile},
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
		RandomID: generateRandomID(),
		Message:  caption,
	}
}

func (c *MTProtoClient) buildVideoMedia(inputFile tg.InputFileClass, caption string) tg.InputSingleMedia {
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
	media, err := c.api.MessagesUploadMedia(c.ctx, &tg.MessagesUploadMediaRequest{
		Peer: &tg.InputPeerSelf{},
		Media: &tg.InputMediaUploadedDocument{
			File:       inputFile,
			MimeType:   getMimeType(filepath.Ext(fileName)),
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
		RandomID: generateRandomID(),
		Message:  caption,
	}
}

// Close gracefully closes the MTProto client
func (c *MTProtoClient) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// resolvePeer converts a chat ID to an InputPeerClass by fetching dialogs
func (c *MTProtoClient) resolvePeer(chatID int64) (tg.InputPeerClass, error) {
	// Get dialogs to find the peer with access hash
	dialogs, err := c.api.MessagesGetDialogs(c.ctx, &tg.MessagesGetDialogsRequest{
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

// Helper: Extract message ID from Updates
func extractMessageID(updates tg.UpdatesClass) int {
	switch u := updates.(type) {
	case *tg.Updates:
		for _, update := range u.Updates {
			if msg, ok := update.(*tg.UpdateNewMessage); ok {
				if m, ok := msg.Message.(*tg.Message); ok {
					return m.ID
				}
			}
			if msg, ok := update.(*tg.UpdateNewChannelMessage); ok {
				if m, ok := msg.Message.(*tg.Message); ok {
					return m.ID
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		return u.ID
	}
	return 0
}

// Helper: Get MIME type from extension
func getMimeType(ext string) string {
	mimeTypes := map[string]string{
		// Images
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".bmp":  "image/bmp",
		// Videos
		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".mkv":  "video/x-matroska",
		".webm": "video/webm",
		".flv":  "video/x-flv",
		// Audio
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".m4a":  "audio/mp4",
		".flac": "audio/flac",
		".aac":  "audio/aac",
		// Documents
		".pdf": "application/pdf",
		".zip": "application/zip",
		".txt": "text/plain",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// Helper: Create proxy dialer from URL
func createProxyDialer(proxyURL string) (proxy.ContextDialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	switch u.Scheme {
	case "socks5":
		// SOCKS5 proxy
		var auth *proxy.Auth
		if u.User != nil {
			password, _ := u.User.Password()
			auth = &proxy.Auth{
				User:     u.User.Username(),
				Password: password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}

		// Wrap to support DialContext
		return &contextDialer{Dialer: dialer}, nil

	case "http", "https":
		// HTTP proxy
		return &httpProxyDialer{
			proxyURL: u,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s (use socks5 or http)", u.Scheme)
	}
}

// contextDialer wraps a proxy.Dialer to implement proxy.ContextDialer
type contextDialer struct {
	Dialer proxy.Dialer
}

func (d *contextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.Dialer.Dial(network, addr)
}

// httpProxyDialer implements proxy.ContextDialer for HTTP proxies
type httpProxyDialer struct {
	proxyURL *url.URL
}

func (d *httpProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Create a dialer with proxy
	dialer := &net.Dialer{}

	// First connect to proxy
	proxyConn, err := dialer.DialContext(ctx, "tcp", d.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Send HTTP CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("failed to send CONNECT: %w", err)
	}

	// Read response (simplified - just check for 200)
	buf := make([]byte, 1024)
	n, err := proxyConn.Read(buf)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("failed to read proxy response: %w", err)
	}

	// Check for success (HTTP/1.1 200 or HTTP/1.0 200)
	response := string(buf[:n])
	if !strings.Contains(response, "200") {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", response)
	}

	return proxyConn, nil
}

// generateRandomID generates a random int64 for message random_id
func generateRandomID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback to a simple random (shouldn't happen)
		return int64(binary.BigEndian.Uint64(buf[:]))
	}
	return int64(binary.BigEndian.Uint64(buf[:]))
}

// isVideoExtension checks if the file extension is a video format
func isVideoExtension(ext string) bool {
	videoExts := map[string]bool{
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".mkv":  true,
		".webm": true,
		".flv":  true,
	}
	return videoExts[ext]
}
