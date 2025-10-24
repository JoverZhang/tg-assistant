# Telegram Uploader Specification

## Overview
A write-only Telegram client that uploads files from a local directory to a specified Telegram chat, then moves processed files to a done directory.

## Technology Stack
- **Library**: `gopkg.in/telebot.v4` (same as used in `cmd/server/main.go`)
- **Language**: Go
- **Dependencies**: telebot.v4 for Telegram Bot API interaction

## Requirements

### Command-Line Arguments
The uploader requires four arguments:
1. **-token** - Telegram bot token for authentication
2. **-local-dir** - Source directory path containing files to upload
3. **-done-dir** - Destination directory path for successfully uploaded files
4. **-chat-id** - Target Telegram chat ID where files will be sent (int64)

Example usage:
```bash
./uploader -token="YOUR_BOT_TOKEN" -local-dir="./uploads" -done-dir="./done" -chat-id=123456789
```

### Core Functionality

#### File Processing
- Monitor the local directory for files to upload
- Parse filename format: `TAG_DESCRIPTION.extension`
  - `TAG`: Hashtag/label for categorization (e.g., `aaa`, `123`) - becomes `#TAG` in caption
  - `DESCRIPTION`: File description or name (used in caption)
  - Example: `travel_sunset_beach.jpg`, `work_report_Q1.pdf`, `tutorial_golang.mp4`

#### Upload Workflow
1. Validate directories exist (create done directory if missing)
2. Scan local directory for files (non-recursive, sorted alphabetically)
3. For each file:
   - Parse filename to extract TAG and DESCRIPTION
   - Skip if filename format is invalid (log warning)
   - Determine media type based on file extension:
     - `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp` → Photo
     - `.mp4`, `.avi`, `.mov`, `.mkv` → Video
     - `.mp3`, `.wav`, `.ogg`, `.m4a` → Audio
     - Everything else → Document
   - Build caption: `#TAG DESCRIPTION` (replace underscores in DESCRIPTION with spaces)
   - Upload file to specified chat ID using telebot.v4
   - On successful upload:
     - Get message ID from upload response
     - Rename file to include message ID: `originalname_msgid_<message_id>.ext`
     - Move renamed file to done directory
   - On failure, log error and continue to next file (do not move)
4. Print final statistics (files processed, succeeded, failed)

#### Client Behavior
- **Write-only**: Never receives or processes incoming messages
- **Non-interactive**: Runs as a batch processor (one-shot execution)
- **No polling**: Initialize bot without `Poller` (no need to call `Start()`)
- Uses direct API calls via `bot.Send()` with appropriate media types

### Architecture

#### Directory Structure
```
tg-assistant/
├── cmd/
│   └── uploader/
│       └── main.go          # Entry point, CLI parsing, orchestration
├── internal/
│   ├── telegram/
│   │   └── uploader.go      # Telegram API client (upload operations)
│   ├── fileprocessor/
│   │   └── processor.go     # File scanning, parsing, moving logic
│   └── config/
│       └── config.go        # Configuration structures
└── UPLOADER_SPEC.md         # This specification
```

#### Component Responsibilities

##### `cmd/uploader/main.go`
- Parse command-line arguments
- Validate inputs (paths exist, token format, etc.)
- Initialize configuration
- Create Telegram client and file processor
- Execute upload workflow
- Handle graceful shutdown

##### `internal/telegram/uploader.go`
- Initialize telebot.v4 Bot instance (without Poller)
- Implement `SendMedia(chatID int64, filePath, caption string) (*tele.Message, error)`
- Determine media type by file extension
- Create appropriate media objects:
  - `&tele.Photo{}` for image files
  - `&tele.Video{}` for video files
  - `&tele.Audio{}` for audio files
  - `&tele.Document{}` for other files
- Use `tele.FromDisk(filePath)` to load files
- Return message response (contains message ID)
- Handle API errors and rate limiting

##### `internal/fileprocessor/processor.go`
- Scan directory for files using `os.ReadDir()` (non-recursive)
- Sort files alphabetically for predictable processing order
- Parse filename format (TAG_DESCRIPTION.ext)
  - Split on first underscore: `parts := strings.SplitN(name, "_", 2)`
  - Extract TAG, DESCRIPTION, and extension
  - Build caption: `#TAG DESCRIPTION` (replace underscores in DESCRIPTION with spaces)
  - Example: `travel_sunset_beach.jpg` → Caption: `#travel sunset beach`
- Coordinate file upload with Telegram uploader
- After successful upload:
  - Extract message ID from response
  - Rename file: `originalname_msgid_<ID>.ext` (e.g., `travel_sunset_msgid_12345.jpg`)
  - Move renamed file to done directory using `os.Rename()` or copy+delete fallback
- Error handling and logging
- Track statistics (processed, succeeded, failed)

##### `internal/config/config.go`
- Define `Config` struct with fields: Token, LocalDir, DoneDir, ChatID
- Implement validation methods
- Parse command-line flags using `flag` package

### File Format Parsing

#### Filename Convention
Format: `TAG_DESCRIPTION.extension`

Examples:
- `travel_sunset_beach.jpg`
  - TAG: "travel", DESCRIPTION: "sunset_beach"
  - Caption: `#travel sunset beach`
  - Media type: Photo (based on .jpg extension)
  - After upload with message ID 12345 → Moved as: `travel_sunset_beach_msgid_12345.jpg`

- `work_quarterly_report.pdf`
  - TAG: "work", DESCRIPTION: "quarterly_report"
  - Caption: `#work quarterly report`
  - Media type: Document (based on .pdf extension)
  - After upload with message ID 67890 → Moved as: `work_quarterly_report_msgid_67890.pdf`

- `tutorial_golang_basics.mp4`
  - TAG: "tutorial", DESCRIPTION: "golang_basics"
  - Caption: `#tutorial golang basics`
  - Media type: Video (based on .mp4 extension)
  - After upload with message ID 54321 → Moved as: `tutorial_golang_basics_msgid_54321.mp4`

#### Parsing Rules
- Split on first underscore to separate TAG from DESCRIPTION
- TAG becomes a hashtag in the caption: `#TAG`
- DESCRIPTION is converted to readable text:
  - Replace underscores with spaces
  - Keep original case
- Caption format: `#TAG DESCRIPTION`
- Example: `travel_sunset_beach.jpg` → Caption: `#travel sunset beach`

#### Media Type Detection (by Extension)
Media type is determined by file extension:

**Photo** (sent as `tele.Photo`):
- `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`, `.bmp`

**Video** (sent as `tele.Video`):
- `.mp4`, `.avi`, `.mov`, `.mkv`, `.webm`, `.flv`

**Audio** (sent as `tele.Audio`):
- `.mp3`, `.wav`, `.ogg`, `.m4a`, `.flac`, `.aac`

**Document** (sent as `tele.Document`):
- All other extensions (`.pdf`, `.zip`, `.txt`, etc.)

### Error Handling
- Invalid filename format: Skip file and log warning
- Upload failure: Log error, optionally retry, do not move to done directory
- Missing directories: Create if possible, otherwise exit with error
- Invalid token or chat ID: Exit with clear error message

### Logging
- Log each file upload attempt (filename, size, result)
- Log errors with context
- Track statistics (files processed, succeeded, failed)

### Implementation Notes for telebot.v4

#### Bot Initialization (Write-Only Mode)
```go
// No Poller needed for write-only client
bot, err := tele.NewBot(tele.Settings{
    Token: token,
    // No Poller - we don't receive messages
})
```

#### Recipient for Sending
```go
// Create recipient from chat ID
recipient := &tele.Chat{ID: chatID}
```

#### File Upload Pattern
```go
// Parse filename
tag, description := parseFilename("travel_sunset_beach.jpg")
// tag = "travel", description = "sunset_beach"

// Build caption: #TAG DESCRIPTION (with spaces)
caption := fmt.Sprintf("#%s %s", tag, strings.ReplaceAll(description, "_", " "))
// caption = "#travel sunset beach"

// Detect media type by extension
ext := strings.ToLower(filepath.Ext(filePath))
file := tele.FromDisk(filePath)

// Send based on media type and capture response
var msg *tele.Message
var err error

switch ext {
case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
    msg, err = bot.Send(recipient, &tele.Photo{
        File:    file,
        Caption: caption,
    })
case ".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv":
    msg, err = bot.Send(recipient, &tele.Video{
        File:    file,
        Caption: caption,
    })
case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac":
    msg, err = bot.Send(recipient, &tele.Audio{
        File:    file,
        Caption: caption,
    })
default:
    msg, err = bot.Send(recipient, &tele.Document{
        File:    file,
        Caption: caption,
    })
}

// On success, extract message ID for filename
if err == nil && msg != nil {
    messageID := msg.ID  // Use this to rename file
    // Example: travel_sunset_beach.jpg → travel_sunset_beach_msgid_12345.jpg
}
```

### Future Enhancements (Optional)
- Watch mode: Monitor directory continuously for new files
- Retry mechanism with exponential backoff
- Progress tracking for large files
- Configuration file support (instead of CLI args only)
- Batch processing with concurrency control
- Dry-run mode to preview what would be uploaded
- File filtering by extension or pattern
