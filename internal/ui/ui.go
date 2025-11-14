package ui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tg-storage-assistant/internal/util"
	"time"

	"github.com/gotd/td/telegram/uploader"
	"github.com/schollz/progressbar/v3"
)

type UploadProgress struct {
	mu          sync.Mutex
	bars        map[int64]*progressbar.ProgressBar // upload ID -> bar
	last        map[int64]int64                    // upload ID -> last uploaded bytes
	initedTotal map[int64]bool                     // whether bar created with known total
}

func NewUploadProgress() *UploadProgress {
	return &UploadProgress{
		bars:        make(map[int64]*progressbar.ProgressBar),
		last:        make(map[int64]int64),
		initedTotal: make(map[int64]bool),
	}
}

func (p *UploadProgress) Chunk(ctx context.Context, st uploader.ProgressState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create bar when we know total size (>=0). If total is not known, wait.
	bar, ok := p.bars[st.ID]
	if !ok && st.Total >= 0 {
		desc := fmt.Sprintf("Uploading [%s]", util.SafeBase(st.Name))
		bar = progressbar.NewOptions64(
			st.Total,
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionSetPredictTime(true),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetDescription(desc),
			progressbar.OptionSetWriter(os.Stderr),
			// progressbar.OptionClearOnFinish(),
		)
		p.bars[st.ID] = bar
		p.initedTotal[st.ID] = true
		p.last[st.ID] = 0
	}

	// If total is not known (stream upload), skip, wait for total to be known before creating bar.
	if bar == nil {
		return nil
	}

	// Incremental update (important! progressbar needs delta)
	prev := p.last[st.ID]
	delta := st.Uploaded - prev
	if delta > 0 {
		_ = bar.Add64(delta)
		p.last[st.ID] = st.Uploaded
	}

	// Finish: clean up
	if st.Total >= 0 && st.Uploaded >= st.Total {
		bar.Finish()
		fmt.Fprintln(os.Stderr)
		delete(p.bars, st.ID)
		delete(p.last, st.ID)
		delete(p.initedTotal, st.ID)
	}
	return nil
}
