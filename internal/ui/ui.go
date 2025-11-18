package ui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"tg-storage-assistant/internal/util"
	"time"

	"github.com/gotd/td/telegram/uploader"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type UploadProgress struct {
	mu       sync.Mutex
	p        *mpb.Progress
	bars     map[int64]*mpb.Bar // upload ID -> bar
	last     map[int64]int64    // upload ID -> last uploaded bytes
	lastTime map[int64]time.Time
}

func NewUploadProgress() *UploadProgress {
	return &UploadProgress{
		p: mpb.New(
			mpb.WithOutput(os.Stderr),
			mpb.WithWidth(60),
		),
		bars:     make(map[int64]*mpb.Bar),
		last:     make(map[int64]int64),
		lastTime: make(map[int64]time.Time),
	}
}

func (p *UploadProgress) Chunk(ctx context.Context, st uploader.ProgressState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	bar, ok := p.bars[st.ID]
	if !ok && st.Total > 0 {
		name := util.SafeBase(st.Name)

		bar = p.p.New(
			st.Total,
			mpb.BarStyle().Lbound("|").Rbound("|").Filler("█").Tip("█").Padding(" ").Refiller(" "),
			mpb.PrependDecorators(
				decor.Name(
					fmt.Sprintf("Uploading %-25s ", "["+name+"]"),
					decor.WC{W: 35, C: decor.DSyncWidthR},
				),
				decor.Percentage(decor.WC{W: 6}),
			),
			mpb.AppendDecorators(
				decor.CountersKibiByte("% .2f / % .2f"),

				decor.Name(" ", decor.WC{W: 1}),
				decor.EwmaSpeed(decor.SizeB1000(0), "(% .2f)", 10,
					decor.WC{W: 10}),

				decor.Name(" ", decor.WC{W: 1}),
				decor.OnComplete(
					decor.EwmaETA(decor.ET_STYLE_GO, 10),
					"✅",
				),
			),
		)

		p.bars[st.ID] = bar
		p.last[st.ID] = 0
		p.lastTime[st.ID] = time.Now()
	}

	if bar == nil {
		return nil
	}

	prev := p.last[st.ID]
	delta := st.Uploaded - prev
	if delta > 0 {
		now := time.Now()
		iterDur := now.Sub(p.lastTime[st.ID])
		// prevent 0, avoid ETA jitter
		if iterDur <= 0 {
			iterDur = time.Millisecond
		}

		bar.EwmaIncrBy(int(delta), iterDur)

		p.last[st.ID] = st.Uploaded
		p.lastTime[st.ID] = now
	}

	if st.Total > 0 && st.Uploaded >= st.Total {
		bar.SetTotal(st.Total, true)
		delete(p.bars, st.ID)
		delete(p.last, st.ID)
		delete(p.lastTime, st.ID)
	}

	return nil
}

func (p *UploadProgress) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, bar := range p.bars {
		bar.Abort(true)
	}
	p.p.Wait()
}
