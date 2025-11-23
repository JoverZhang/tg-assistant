package main

import (
	"context"
	"fmt"
	"log"
	"tg-storage-assistant/internal/client"
	"tg-storage-assistant/internal/config"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Config string `help:"Path to config file" short:"f" default:"config.yaml"`

	History HistoryCmd `cmd:"" help:"Show history of chat"`
}

type HistoryCmd struct {
	ChatID   int64 `help:"Chat ID" short:"c" required:"true"`
	OffsetID int   `help:"Offset ID" short:"o" default:"0"`
	Limit    int   `help:"Limit" short:"l" default:"20"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	cfg, err := config.LoadConfig(cli.Config)
	if err != nil {
		log.Fatal(err)
	}

	switch ctx.Command() {
	case "history":
		if err := cli.History.Run(&cfg.Mtproto); err != nil {
			log.Fatal(err)
		}
	}
}

func (h *HistoryCmd) Run(cfg *config.MtprotoConfig) error {
	ctx := context.Background()

	cl, err := client.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("new client failed: %v", err)
	}

	err = cl.Run(func(ctx context.Context) error {
		msgs, err := cl.GetHistory(h.ChatID, client.HistoryOptions{
			OffsetID: h.OffsetID,
			Limit:    h.Limit,
		})
		if err != nil {
			return err
		}

		if len(msgs) == 0 {
			fmt.Println("no messages found")
			return nil
		}

		fmt.Printf("page has %d messages\n", len(msgs))
		for _, m := range msgs {
			// t := time.Unix(int64(m.Date), 0)
			fmt.Println(m.Message)
			// fmt.Printf(
			// 	"- id=%d date=%s from=%v text=%q\n",
			// 	m.ID,
			// 	t.Format("2006-01-02 15:04:05"),
			// 	m.FromID,
			// 	m.Message,
			// )
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("run failed: %w", err)
	}
	return nil
}
