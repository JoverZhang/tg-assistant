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
5. **-max-size** - Maximum file size for video splitting (e.g., "2G", "500M", "1.5G"). Optional, defaults to no splitting.

Example usage:
```bash
./uploader -token="YOUR_BOT_TOKEN" -local-dir="./uploads" -done-dir="./done" -chat-id=123456789 -max-size="2G"
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
   - **For video files**: Apply video processing workflow (see "Video Processing and Splitting" section)
   - **For non-video files**: Upload directly to specified chat ID using telebot.v4
   - On successful upload:
     - Get message ID from upload response
     - Rename file to include message ID: `originalname_msgid_<message_id>.ext`
     - Move renamed file to done directory
   - On failure, log error and continue to next file (do not move)
4. Print final statistics (files processed, succeeded, failed)

#### Video Processing and Splitting

**All video files** undergo the following processing workflow:

1. **Generate Preview Photo (Always)**:
   - Extract 30 frames evenly distributed across the full video timeline
   - Use ffmpeg to extract frames at calculated timestamps
   - Compose frames into a 5×6 grid (6 columns, 5 rows) as a single JPEG image
   - Save preview as `TAG_DESCRIPTION_preview.jpg` in a temporary location
   - Preview image will be the first item in the media group

2. **Check File Size for Splitting** (if `-max-size` is specified):
   - If video size ≤ max-size: Upload as single video + preview photo (2 items in media group)
   - If video size > max-size: Apply splitting workflow below

3. **Video Splitting Workflow** (for videos exceeding max-size):
   - Calculate number of chunks: `ceil(fileSize / maxSize)`
   - Split video into chunks using ffmpeg (e.g., 6.5G with max-size=2G → 4 chunks: 2G, 2G, 2G, 0.5G)
   - Name chunks: `TAG_DESCRIPTION_part1.ext`, `TAG_DESCRIPTION_part2.ext`, etc.
   - Save chunks to temporary location
   - Verify Telegram media group limit (max 10 items):
     - Total items = 1 preview + N chunks
     - If total > 10: Log error and skip file (do not upload)
     - If total ≤ 10: Proceed to upload

4. **Upload as Media Group/Album**:
   - Create media group with:
     - **First item**: Preview photo (`tele.Photo`) with caption `#TAG DESCRIPTION` (this is the only caption for the entire album)
     - **Remaining items**: Video chunks (`tele.Video`) with empty captions (Telegram only shows the first item's caption for the entire album)
   - Use `bot.SendAlbum(recipient, album)` to send as single media group
   - On successful upload:
     - Get message ID from first item in response
     - Rename and move files to done directory:
       - Original video: `originalname_msgid_<message_id>.ext`
       - Preview photo: `originalname_preview_msgid_<message_id>.jpg`
       - Video chunks (if split): `originalname_part1_msgid_<message_id>.ext`, etc.
   - On failure: Log error, delete temporary files, keep original in local directory

5. **Cleanup**:
   - Delete temporary split chunks and preview from temp location
   - Original video file remains in local directory (will be moved to done on success)

**Example Scenarios**:

- **Small video (1.5G, max-size=2G)**:
  - Generate 5×6 preview (30 frames)
  - Media group: [Preview photo, Original video] = 2 items
  - Album caption: `#travel vacation moments` (shown on preview photo)

- **Large video (6.5G, max-size=2G)**:
  - Generate 5×6 preview (30 frames from full 6.5G video)
  - Split into 4 chunks: 2G, 2G, 2G, 0.5G
  - Media group: [Preview, Part1, Part2, Part3, Part4] = 5 items
  - Album caption: `#tutorial golang advanced` (shown on preview photo, applies to entire album)

- **Huge video (25G, max-size=2G)**:
  - Would create 13 chunks + 1 preview = 14 items
  - **Exceeds Telegram limit (10 items)**: Log error, skip file, do not upload

#### Client Behavior
- **Write-only**: Never receives or processes incoming messages
- **Non-interactive**: Runs as a batch processor (one-shot execution)
- **No polling**: Initialize bot without `Poller` (no need to call `Start()`)
- Uses direct API calls via `bot.Send()` for single files or `bot.SendAlbum()` for media groups

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
│   ├── video/
│   │   ├── splitter.go      # Video splitting logic (ffmpeg wrapper)
│   │   └── thumbnail.go     # Frame extraction and grid composition
│   └── config/
│       └── config.go        # Configuration structures
└── spec/
    └── uploader/
        └── spec.md          # This specification
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
- Implement `SendMedia(chatID int64, filePath, caption string) (*tele.Message, error)` for single files
- Implement `SendMediaGroup(chatID int64, items []MediaItem) ([]*tele.Message, error)` for albums
  - MediaItem contains: filePath, mediaType (photo/video), caption
  - Build `tele.Album` from items
  - Use `bot.SendAlbum(recipient, album)` to send as single media group
  - Return array of message responses (first message contains primary message ID)
- Determine media type by file extension
- Create appropriate media objects:
  - `&tele.Photo{}` for image files
  - `&tele.Video{}` for video files
  - `&tele.Audio{}` for audio files
  - `&tele.Document{}` for other files
- Use `tele.FromDisk(filePath)` to load files
- Handle API errors and rate limiting

##### `internal/fileprocessor/processor.go`
- Scan directory for files using `os.ReadDir()` (non-recursive)
- Sort files alphabetically for predictable processing order
- Parse filename format (TAG_DESCRIPTION.ext)
  - Split on first underscore: `parts := strings.SplitN(name, "_", 2)`
  - Extract TAG, DESCRIPTION, and extension
  - Build caption: `#TAG DESCRIPTION` (replace underscores in DESCRIPTION with spaces)
  - Example: `travel_sunset_beach.jpg` → Caption: `#travel sunset beach`
- Route file processing based on media type:
  - **Videos**: Call video processing workflow
    - Generate preview thumbnail (5×6 grid, 30 frames)
    - Check size and split if needed (using video/splitter.go)
    - Create media group items (preview + video chunks)
    - Upload as media group via `SendMediaGroup()`
    - Clean up temporary files (split chunks, preview in temp)
    - Move original video and save preview to done directory with message ID
  - **Non-videos**: Direct upload via `SendMedia()`
- After successful upload:
  - Extract message ID from response (first message for media groups)
  - Rename files:
    - Single file: `originalname_msgid_<ID>.ext`
    - Video with preview: `originalname_msgid_<ID>.ext` + `originalname_preview_msgid_<ID>.jpg`
    - Split video: `originalname_msgid_<ID>.ext` + `originalname_part1_msgid_<ID>.ext` + ... + preview
  - Move renamed files to done directory using `os.Rename()` or copy+delete fallback
- Error handling and logging
- Track statistics (processed, succeeded, failed)

##### `internal/config/config.go`
- Define `Config` struct with fields: Token, LocalDir, DoneDir, ChatID, MaxSize (in bytes)
- Implement validation methods
- Parse command-line flags using `flag` package
- Parse size string (e.g., "2G", "500M") to bytes

##### `internal/video/thumbnail.go`
- Extract N frames evenly distributed from video using ffmpeg
- Implementation: `ExtractFrames(videoPath string, count int, outputDir string) ([]string, error)`
  - Calculate timestamps: for 30 frames in 60s video → extract at 0s, 2s, 4s, ..., 58s
  - Use ffmpeg command: `ffmpeg -i input.mp4 -vf "select='eq(n\,FRAME_NUMBER)'" -vframes 1 frame_%03d.jpg`
  - Return paths to extracted frame images
- Compose frames into grid: `ComposeGrid(framePaths []string, cols, rows int, outputPath string) error`
  - Use Go image libraries (`image`, `image/jpeg`, `image/draw`)
  - Arrange 30 frames in 6×5 grid (6 columns, 5 rows)
  - Save as JPEG with reasonable quality (e.g., 85%)
- Clean up temporary frame files after grid composition

##### `internal/video/splitter.go`
- Check video file size and split if needed
- Implementation: `SplitVideo(videoPath string, maxSize int64, outputDir string) ([]string, error)`
  - Calculate chunk size and count
  - Use ffmpeg to split video by size:
    - `ffmpeg -i input.mp4 -c copy -f segment -segment_time_delta 0.1 -segment_size <maxSize> output_part%03d.mp4`
  - Alternative: Split by duration for more precise control
  - Return paths to split video files (or single original if no split needed)
- Validate chunk count doesn't exceed Telegram limits (preview + chunks ≤ 10)
- Handle cleanup of temporary split files on error

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
- Video processing errors:
  - ffmpeg not available: Exit with error message (require ffmpeg in PATH)
  - Frame extraction failure: Log error, skip file
  - Split failure: Log error, clean up temporary files, skip file
  - Media group exceeds 10 items: Log error, skip file (suggest larger max-size)
- Temporary file cleanup: Always clean up temp files on error or after successful upload

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

#### Single File Upload Pattern
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
    // Videos go through video processing workflow (see below)
    // This is for non-video files only
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

#### Video Upload with Preview and Splitting Pattern
```go
// Example: Processing "tutorial_golang_basics.mp4" (6.5G, max-size=2G)
tag := "tutorial"
description := "golang_basics"
baseCaption := "#tutorial golang basics"

// Step 1: Generate preview thumbnail
tempDir := "/tmp/uploader_video_processing"
frames, err := video.ExtractFrames(videoPath, 30, tempDir)
if err != nil { /* handle error */ }

previewPath := filepath.Join(tempDir, "preview.jpg")
err = video.ComposeGrid(frames, 6, 5, previewPath) // 6 cols, 5 rows
if err != nil { /* handle error */ }

// Step 2: Split video if needed
videoParts, err := video.SplitVideo(videoPath, maxSizeBytes, tempDir)
if err != nil { /* handle error */ }
// Returns: [part1.mp4, part2.mp4, part3.mp4, part4.mp4] or [original.mp4] if no split

// Step 3: Validate media group size
totalItems := 1 + len(videoParts) // preview + video parts
if totalItems > 10 {
    // Error: exceeds Telegram limit
    // Clean up temp files and skip
}

// Step 4: Build media group
album := tele.Album{}

// First item: preview photo with caption (only caption for entire album)
album = append(album, &tele.Photo{
    File:    tele.FromDisk(previewPath),
    Caption: baseCaption, // This caption applies to the entire album
})

// Remaining items: video parts with empty captions
for _, partPath := range videoParts {
    album = append(album, &tele.Video{
        File:    tele.FromDisk(partPath),
        Caption: "", // Empty caption - Telegram only shows first item's caption
    })
}

// Step 5: Send media group
messages, err := bot.SendAlbum(recipient, album)
if err != nil {
    // Clean up temp files and handle error
}

// Step 6: Extract message ID (from first message in group)
messageID := messages[0].ID

// Step 7: Move files to done directory with message ID
// - Original video: tutorial_golang_basics_msgid_12345.mp4
// - Preview: tutorial_golang_basics_preview_msgid_12345.jpg
// - Parts (optional): tutorial_golang_basics_part1_msgid_12345.mp4, part2, etc.

// Step 8: Clean up temp directory
os.RemoveAll(tempDir)
```

#### ffmpeg Commands Reference

**Extract frames at specific timestamps**:
```bash
# Extract 30 frames evenly from a 60-second video
# Calculate interval: 60s / 30 frames = 2s per frame
ffmpeg -i input.mp4 -vf "fps=1/2" -frames:v 30 frame_%03d.jpg
# Or extract at exact timestamps:
ffmpeg -ss 00:00:00 -i input.mp4 -vframes 1 frame_001.jpg
ffmpeg -ss 00:00:02 -i input.mp4 -vframes 1 frame_002.jpg
# ... (repeat for each timestamp)
```

**Split video by size**:
```bash
# Split into chunks of max 2GB each
ffmpeg -i input.mp4 -c copy -map 0 -f segment -segment_size 2G output_part%03d.mp4
# Output: output_part000.mp4, output_part001.mp4, etc.
```

**Get video duration** (for calculating frame extraction timestamps):
```bash
ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 input.mp4
# Output: 60.5 (seconds)
```

### Dependencies
- **telebot.v4**: Telegram Bot API library (`gopkg.in/telebot.v4`)
- **ffmpeg**: Required for video processing (frame extraction and splitting)
  - Must be available in system PATH
  - Check on startup: `exec.LookPath("ffmpeg")`
- **ffprobe**: Required for video metadata (duration, etc.)
  - Usually bundled with ffmpeg
- **Go standard library**:
  - `flag`: CLI argument parsing
  - `os`, `path/filepath`: File operations
  - `image`, `image/jpeg`, `image/draw`: Image composition for grid
  - `os/exec`: Execute ffmpeg commands
  - `fmt`, `strings`: String formatting

### Future Enhancements (Optional)
- Watch mode: Monitor directory continuously for new files
- Retry mechanism with exponential backoff
- Progress tracking for large files and video processing operations
- Configuration file support (instead of CLI args only)
- Batch processing with concurrency control
- Dry-run mode to preview what would be uploaded
- File filtering by extension or pattern
- Custom grid dimensions (e.g., 4×8, 3×10) via CLI argument
- GPU-accelerated video processing (ffmpeg with NVENC/VAAPI)
- Smart frame selection (scene change detection instead of uniform distribution)
- Configurable preview quality and resolution
