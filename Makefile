-include .env
export $(shell sed -n 's/\r//;/^[[:space:]]*#/d;/^[[:space:]]*$$/d;s/^\([^=[:space:]]\+\)=.*/\1/p' .env)

SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c

init-test-uploader:
	@echo "Initializing uploader..."
	# Prepare the test directory
	rm -rf /tmp/test-uploader
	mkdir -p /tmp/test-uploader/{done,local}
	# Create test video
	ffmpeg -f lavfi -i smptehdbars=size=1280x720:rate=30 \
	       -f lavfi -i sine=frequency=1000:sample_rate=44100 \
	       -c:v libx264 -pix_fmt yuv420p -b:v 2M \
	       -c:a aac -b:a 128k -t 45 \
	       -movflags +faststart /tmp/test-uploader/local/test_11mb.mp4
	# Create test photo with better quality settings
	ffmpeg -f lavfi -i color=red:size=1280x720:rate=1 -frames:v 1 -q:v 2 /tmp/test-uploader/local/test_photo.jpg
	# Create test files for media group
	ffmpeg -f lavfi -i color=blue:size=1280x720:rate=1 -frames:v 1 -q:v 2 /tmp/test-uploader/local/test_preview.jpg
	ffmpeg -i /tmp/test-uploader/local/test_11mb.mp4 -c copy -t 22 /tmp/test-uploader/local/test_part1.mp4
	ffmpeg -i /tmp/test-uploader/local/test_11mb.mp4 -c copy -ss 22 /tmp/test-uploader/local/test_part2.mp4
	cp /tmp/big_medias/68MB.mov /tmp/test-uploader/local/test_68mb.mov
	@echo "✓ Test files created"

test-upload:
	go test -v ./internal/telegram -run TestUpload

run-test-uploader2:
	@echo "Running test uploader2..."
	go run ./cmd/uploader2 \
		-api-id="$(API_ID)" \
		-api-hash="$(API_HASH)" \
		-phone="$(PHONE)" \
		-local-dir="/tmp/test-uploader/local" \
		-temp-dir="/tmp/test-uploader/temp" \
		-done-dir="/tmp/test-uploader/done" \
		-storage-chat-id="$(CHAT_ID)" \
		-proxy="$(PROXY_URL)" \
		-max-size="20MB" \
		-cleanup-temp-dir=false

build-uploader2:
	@echo "Building uploader2 binary..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/uploader2 ./cmd/uploader2
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ./bin/uploader2.exe ./cmd/uploader2

build-uploader2-and-upload: build-uploader2
	@echo "Building uploader2 and uploading files..."
	mcli cp ./bin/uploader2 ./bin/uploader2.exe singapore/test-tg-assistant

build-docker-image: build-uploader2
	@echo "Building docker image..."
	docker build \
		--build-arg APT_PROXY="$(PROXY_URL)" \
		-t joverzhang/tg-assistant-uploader:0.1 \
		--network host \
		.

run-test-docker-image: build-docker-image
	@echo "Running test docker image..."
	docker run --rm -it \
		-v ./session.json:/session/session.json \
		-v /tmp/test-uploader:/data \
		--network host \
		joverzhang/tg-assistant-uploader:0.1 \
		-session-file="/session/session.json" \
		-api-id="$(API_ID)" \
		-api-hash="$(API_HASH)" \
		-phone="$(PHONE)" \
		-local-dir="/data/local" \
		-temp-dir="/data/temp" \
		-done-dir="/data/done" \
		-storage-chat-id="$(CHAT_ID)" \
		-proxy="$(PROXY_URL)" \
		-max-size="20MB" \
		-cleanup-temp-dir=false
