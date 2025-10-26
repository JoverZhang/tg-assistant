# Telegram Uploader Specification

## Overview
A write-only Telegram client that uploads files from a local directory to a storage chat using MTProto (user account), then moves processed files to a done directory. Uses MTProto to bypass the 50MB Bot API file size limit, supporting files up to 2GB.

## Technology Stack
- **Library**: `github.com/gotd/td` (MTProto user client)
- **Language**: Go
- **Dependencies**:
  - gotd/td for MTProto user client
  - ffmpeg/ffprobe for video processing
  - golang.org/x/image for image composition

## Requirements

### Command-Line Arguments
The uploader requires the following arguments:
1. **-session-file** - Path to MTProto session file (e.g., "./session.json"). Will be created on first run.
2. **-api-id** - Telegram API ID (obtain from https://my.telegram.org/apps)
3. **-api-hash** - Telegram API hash (obtain from https://my.telegram.org/apps)
4. **-phone** - Phone number for authentication (e.g., "+1234567890"). Only needed on first run.
5. **-local-dir** - Source directory path containing files to upload
6. **-done-dir** - Destination directory path for successfully uploaded files
7. **-storage-chat-id** - Storage chat ID where files will be uploaded (int64)
8. **-max-size** - Maximum file size for video splitting (e.g., "2G", "500M", "1.5G"). Optional, defaults to no splitting.
9. **-proxy** - Proxy server URL (e.g., "socks5://127.0.0.1:1080" or "http://127.0.0.1:8080"). Optional.

Example usage (first run with authentication):
```bash
./uploader \
  -session-file="./session.json" \
  -api-id="12345" \
  -api-hash="abcdef123456" \
  -phone="+1234567890" \
  -local-dir="./uploads" \
  -done-dir="./done" \
  -storage-chat-id=-100123456789 \
  -max-size="2G"
```

Example usage (subsequent runs, using saved session):
```bash
./uploader \
  -session-file="./session.json" \
  -api-id="12345" \
  -api-hash="abcdef123456" \
  -local-dir="./uploads" \
  -done-dir="./done" \
  -storage-chat-id=-100123456789 \
  -max-size="2G"
```

Example usage (with SOCKS5 proxy):
```bash
./uploader \
  -session-file="./session.json" \
  -api-id="12345" \
  -api-hash="abcdef123456" \
  -local-dir="./uploads" \
  -done-dir="./done" \
  -storage-chat-id=-100123456789 \
  -proxy="socks5://127.0.0.1:1080" \
  -max-size="2G"
```

Example usage (with HTTP proxy):
```bash
./uploader \
  -session-file="./session.json" \
  -api-id="12345" \
  -api-hash="abcdef123456" \
  -local-dir="./uploads" \
  -done-dir="./done" \
  -storage-chat-id=-100123456789 \
  -proxy="http://127.0.0.1:8080" \
  -max-size="2G"
```

**Note**: On first run, the user will be prompted to enter the authentication code sent to their phone.

### Prerequisites

#### Telegram API Credentials
To use MTProto, you need to obtain API credentials:
1. Visit https://my.telegram.org/apps
2. Log in with your phone number
3. Create a new application
4. Note down the `api_id` (integer) and `api_hash` (string)

#### Storage Chat Setup
Create a dedicated chat for file storage:
- **Option 1: Private Channel** (Recommended)
  - Create a private channel
  - Add your user account as admin
  - Get the channel ID (e.g., `-100123456789`)
  - This chat will store all uploaded files

- **Option 2: Private Group**
  - Create a private group
  - Add your user account
  - Get the group ID

- **Option 3: Saved Messages**
  - Use your own "Saved Messages" chat
  - Get your user ID as the chat ID

**Note**: If using a bot for retrieval (see server component), the bot must also be added to this storage chat.

#### Phone Number
- A valid Telegram phone number registered to your account
- Will receive authentication code on first run
- Session is saved after authentication, no need to re-authenticate

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
   - **For non-video files**: Upload directly to storage chat using MTProto
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
     - **First item**: Preview photo with caption `#TAG DESCRIPTION` (this is the only caption for the entire album)
     - **Remaining items**: Video chunks with empty captions (Telegram only shows the first item's caption for the entire album)
   - Use MTProto `messages.SendMultiMedia` to send as single media group
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
- **MTProto-based**: Uses user account authentication via gotd/td
- **Session persistence**: Saves session to file after first authentication
- Uses MTProto API calls via `messages.SendMedia` for single files or `messages.SendMultiMedia` for media groups

### Architecture

#### Directory Structure
```
tg-assistant/
├── cmd/
│   └── uploader/
│       └── main.go          # Entry point, CLI parsing, orchestration
├── internal/
│   ├── telegram/
│   │   ├── uploader.go      # Legacy Bot API uploader (deprecated)
│   │   └── mtproto.go       # MTProto user client (upload operations)
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
- Parse command-line arguments (including MTProto credentials)
- Validate inputs (paths exist, API credentials, etc.)
- Initialize configuration
- Create MTProto client and file processor
- Handle first-time authentication (phone code verification)
- Execute upload workflow
- Handle graceful shutdown and session saving

##### `internal/telegram/mtproto.go`
- Initialize gotd/td client with session management
- Implement `NewMTProtoClient(sessionFile, apiID, apiHash, phone string) (*MTProtoClient, error)`
  - Load existing session from file if available
  - Otherwise, perform authentication flow (phone + code)
  - Save session to file after successful authentication
- Implement `SendMedia(chatID int64, filePath, caption string) (int, error)` for single files
  - Returns message ID on success
- Implement `SendMediaGroup(chatID int64, items []MediaItem) (int, error)` for albums
  - MediaItem contains: filePath, mediaType (photo/video), caption
  - Build InputMedia array from items
  - Use `messages.SendMultiMedia` to send as single media group
  - Return message ID from first item in response
- Determine media type by file extension
- Create appropriate InputMedia objects:
  - `InputMediaUploadedPhoto` for image files
  - `InputMediaUploadedDocument` for video/audio/document files
- Upload file chunks for large files using `upload.SaveBigFilePart`
- Handle MTProto errors and rate limiting
- Implement session persistence (save/load from JSON file)

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
- Define `Config` struct with fields:
  - SessionFile: string (path to session file)
  - APIID: int (Telegram API ID)
  - APIHash: string (Telegram API hash)
  - Phone: string (phone number for authentication)
  - LocalDir: string (source directory)
  - DoneDir: string (destination directory)
  - StorageChatID: int64 (storage chat ID)
  - MaxSize: int64 (maximum file size in bytes for splitting)
- Implement validation methods
- Parse command-line flags using `flag` package
- Parse size string (e.g., "2G", "500M") to bytes

##### `internal/video/thumbnail.go`
- Extract N frames evenly distributed from video using ffmpeg
- Implementation: `ExtractFrames(videoPath string, count int, outputDir string) ([]string, error)`
  - Get video duration using ffprobe
  - Calculate timestamps: for 30 frames in 60s video → extract at 0s, 2s, 4s, ..., 58s
  - Use ffmpeg command: `ffmpeg -ss <timestamp> -i input.mp4 -vframes 1 -q:v 2 frame_%03d.jpg`
  - Return paths to extracted frame images
- Compose frames into grid: `ComposeGrid(framePaths []string, cols, rows int, outputPath string) error`
  - Use Go image libraries (`image`, `image/jpeg`, `image/draw`, `golang.org/x/image/draw`)
  - Load each frame and resize to thumbnail size (~320px width to avoid exceeding Telegram's dimension limits)
  - Arrange 30 frames in 6×5 grid (6 columns, 5 rows)
  - Final grid dimensions: ~1920×900 pixels (suitable for Telegram)
  - Use bilinear interpolation for smooth scaling
  - Save as JPEG with reasonable quality (e.g., 85%)
- Helper: `getVideoDuration(videoPath string) (float64, error)` - Gets video duration using ffprobe
- Clean up temporary frame files after grid composition

##### `internal/video/splitter.go`
- Check video file size and split if needed
- Implementation: `SplitVideo(videoPath string, maxSize int64, outputDir string) ([]string, error)`
  - Get file size and check if splitting is needed
  - If file ≤ maxSize or maxSize not specified, return original video path
  - Get video duration using `getVideoDuration()` (from thumbnail.go)
  - Calculate bitrate: `bitrate = fileSize / duration` (bytes per second)
  - Calculate chunk duration: `chunkDuration = maxSize / bitrate` (seconds per chunk)
  - Split by time segments using ffmpeg (more reliable than byte-based splitting):
    - `ffmpeg -i input.mp4 -c copy -map 0 -f segment -segment_time <chunkDuration> -reset_timestamps 1 output_part%03d.mp4`
  - Capture ffmpeg output for error debugging
  - Return paths to split video files (or single original if no split needed)
- Helper: `ValidateChunkCount(numVideoParts int) error` - Validates chunk count doesn't exceed Telegram limits (preview + chunks ≤ 10)
- Helper: `CleanupTempFiles(paths []string) error` - Handles cleanup of temporary split files on error

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
- Upload failure: Log error with details (including MTProto error messages), do not move to done directory
- Missing directories: Create if possible, otherwise exit with error
- Invalid API credentials: Exit with clear error message
- Authentication errors:
  - Invalid phone number: Exit with error
  - Auth code timeout: Prompt user to retry
  - Session file corrupted: Delete and re-authenticate
  - Network errors during auth: Retry with backoff
- Storage chat errors:
  - Chat not found: Exit with error (user needs to check chat ID)
  - No access to chat: Exit with error (user account must be member)
  - Chat permissions insufficient: Exit with error
- Video processing errors:
  - ffmpeg/ffprobe not available: Show warning at startup, fail gracefully on video files
  - Frame extraction failure: Log error with ffmpeg output, clean up temp files, skip file
  - Preview composition failure: Log error, clean up temp files, skip file
  - Invalid preview dimensions: Automatically resize frames to ~320px width to avoid Telegram's dimension limits
  - Split failure: Log error with ffmpeg output, clean up temporary files, skip file
  - Media group exceeds 10 items: Log error, skip file (suggest larger max-size)
- MTProto specific errors:
  - FLOOD_WAIT: Respect rate limit, wait and retry
  - FILE_PARTS_INVALID: Log error and skip file
  - PEER_ID_INVALID: Check storage chat ID
- Temporary file cleanup: Always clean up temp files on error or after successful upload

### Logging
- Log each file upload attempt (filename, size, result)
- Log errors with context
- Track statistics (files processed, succeeded, failed)

### Implementation Notes for gotd/td (MTProto)

#### Client Initialization and Authentication
```go
// Initialize client with session management
client, err := telegram.NewMTProtoClient(telegram.MTProtoConfig{
    SessionFile: "./session.json",
    APIID:       12345,
    APIHash:     "abcdef123456",
    Phone:       "+1234567890",
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// On first run, user will be prompted to enter auth code sent to phone
// Session is saved to file for future runs
```

#### Session Management
```go
// Session structure (saved as JSON)
type Session struct {
    AuthKey []byte `json:"auth_key"`
    Salt    int64  `json:"salt"`
}

// Load session from file
func loadSession(path string) (*Session, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var s Session
    err = json.Unmarshal(data, &s)
    return &s, err
}

// Save session to file
func saveSession(path string, s *Session) error {
    data, err := json.Marshal(s)
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0600)
}
```

#### Single File Upload Pattern
```go
// Parse filename
tag, description := parseFilename("travel_sunset_beach.jpg")
// tag = "travel", description = "sunset_beach"

// Build caption: #TAG DESCRIPTION (with spaces)
caption := fmt.Sprintf("#%s %s", tag, strings.ReplaceAll(description, "_", " "))
// caption = "#travel sunset beach"

// Upload using MTProto
messageID, err := client.SendMedia(storageChatID, filePath, caption)
if err != nil {
    log.Printf("Upload failed: %v", err)
    return
}

// On success, rename and move file
newName := fmt.Sprintf("%s_msgid_%d%s", baseFilename, messageID, ext)
// Example: travel_sunset_beach.jpg → travel_sunset_beach_msgid_12345.jpg
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

// Step 4: Build media items
mediaItems := []telegram.MediaItem{
    // First item: preview photo with caption
    {
        FilePath:  previewPath,
        MediaType: "photo",
        Caption:   baseCaption, // This caption applies to the entire album
    },
}

// Remaining items: video parts with empty captions
for _, partPath := range videoParts {
    mediaItems = append(mediaItems, telegram.MediaItem{
        FilePath:  partPath,
        MediaType: "video",
        Caption:   "", // Empty caption
    })
}

// Step 5: Send media group using MTProto
messageID, err := client.SendMediaGroup(storageChatID, mediaItems)
if err != nil {
    // Clean up temp files and handle error
}

// Step 6: Move files to done directory with message ID
// - Original video: tutorial_golang_basics_msgid_12345.mp4
// - Preview: tutorial_golang_basics_preview_msgid_12345.jpg
// - Parts (optional): tutorial_golang_basics_part1_msgid_12345.mp4, part2, etc.

// Step 7: Clean up temp directory
os.RemoveAll(tempDir)
```

#### MTProto Large File Upload Pattern
```go
// For files > 10MB, use upload.SaveBigFilePart
// gotd/td handles this automatically in uploader.Upload()

func (c *MTProtoClient) uploadFile(filePath string) (*tg.InputFile, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    stat, err := f.Stat()
    if err != nil {
        return nil, err
    }

    // Use uploader from gotd/td
    u := uploader.NewUploader(c.api)

    // Upload returns InputFile that can be used in messages.SendMedia
    inputFile, err := u.Upload(context.Background(), uploader.NewUpload(
        filepath.Base(filePath),
        f,
        stat.Size(),
    ))

    return inputFile, err
}
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
- **gotd/td**: Telegram MTProto client library (`github.com/gotd/td`)
  - Core client: `github.com/gotd/td/telegram`
  - File uploads: `github.com/gotd/td/telegram/uploader`
  - TL types: `github.com/gotd/td/tg`
- **golang.org/x/image**: Extended image processing library for high-quality image scaling
  - Used for resizing video frames before composing preview grid
  - Provides bilinear interpolation for smooth thumbnail scaling
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
  - `fmt`, `strings`, `strconv`: String formatting and parsing
  - `math`: Mathematical operations (e.g., chunk count calculation)
  - `encoding/json`: Session persistence
  - `context`: Context management for MTProto operations

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
