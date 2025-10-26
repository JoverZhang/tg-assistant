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
	# Check the test_10mb.mp4 file exists, if not create it
	if [ ! -f ./tests/media/local/test_10mb.mp4 ]; then \
		ffmpeg -f lavfi -i smptehdbars=size=1280x720:rate=30 \
		       -f lavfi -i sine=frequency=1000:sample_rate=44100 \
		       -c:v libx264 -pix_fmt yuv420p -b:v 2M \
		       -c:a aac -b:a 128k -t 45 \
		       -movflags +faststart /tmp/test-uploader/local/test_11mb.mp4 ; \
	fi

run-test-uploader:
	@echo "Running test uploader..."
	go run ./cmd/uploader \
		--token="$(TOKEN)" \
		--local-dir="/tmp/test-uploader/local" \
		--done-dir="/tmp/test-uploader/done" \
		--chat-id="$(CHAT_ID)" \
		--max-size="500KB"

build-uploader:
	@echo "Building uploader binary..."
	go build -o ./bin/uploader ./cmd/uploader
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/uploader.exe ./cmd/uploader
