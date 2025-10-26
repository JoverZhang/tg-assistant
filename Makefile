-include .env
export $(shell sed -n 's/\r//;/^[[:space:]]*#/d;/^[[:space:]]*$$/d;s/^\([^=[:space:]]\+\)=.*/\1/p' .env)

SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c

init-test-uploader:
	@echo "Initializing uploader..."
	# Prepare the test directory
	rm -rf /tmp/test-uploader
	mkdir -p /tmp/test-uploader/{done,local,upload}
	# Create test video
	ffmpeg -f lavfi -i smptehdbars=size=1280x720:rate=30 \
	       -f lavfi -i sine=frequency=1000:sample_rate=44100 \
	       -c:v libx264 -pix_fmt yuv420p -b:v 2M \
	       -c:a aac -b:a 128k -t 45 \
	       -movflags +faststart /tmp/test-uploader/local/test_11mb.mp4
	# Create test photo with better quality settings
	ffmpeg -f lavfi -i color=red:size=1280x720:rate=1 -frames:v 1 -q:v 2 /tmp/test-uploader/upload/test_photo.jpg
	# Create test files for media group
	ffmpeg -f lavfi -i color=blue:size=1280x720:rate=1 -frames:v 1 -q:v 2 /tmp/test-uploader/upload/test_preview.jpg
	ffmpeg -i /tmp/test-uploader/local/test_11mb.mp4 -c copy -t 22 /tmp/test-uploader/upload/test_part1.mp4
	ffmpeg -i /tmp/test-uploader/local/test_11mb.mp4 -c copy -ss 22 /tmp/test-uploader/upload/test_part2.mp4
	@echo "✓ Test files created"

test-upload:
	go test -v ./internal/telegram -run TestUpload

run-test-uploader:
	@echo "Running test uploader..."
	go run ./cmd/uploader \
		-api-id="$(API_ID)" \
		-api-hash="$(API_HASH)" \
		-phone="$(PHONE)" \
		-local-dir="/tmp/test-uploader/local" \
		-done-dir="/tmp/test-uploader/done" \
		-storage-chat-id="$(CHAT_ID)" \
		-proxy="$(PROXY_URL)" \
		-max-size="500KB"

build-uploader:
	@echo "Building uploader binary..."
	go build -o ./bin/uploader ./cmd/uploader
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/uploader.exe ./cmd/uploader
