package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/gateway"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/config"
	"mayuri2000/internal/imagefind"
	"mayuri2000/internal/imaging"
	"mayuri2000/internal/modules/core"
	"mayuri2000/internal/modules/feeds"
	"mayuri2000/internal/modules/moderation"
	"mayuri2000/internal/modules/notsobot"
	"mayuri2000/internal/modules/reminders"
	"mayuri2000/internal/modules/search"
	"mayuri2000/internal/rss"
	"mayuri2000/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	st, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Error("closing database", "err", err)
		}
	}()

	magick, err := imaging.NewMagick(context.Background(), cfg.MagickPath)
	if err != nil {
		log.Warn("ImageMagick unavailable; magik, caption and watermark are disabled", "err", err)
	} else if !magick.LiquidRescale {
		log.Warn("ImageMagick lacks liquid rescale (liblqr) support; magik is disabled")
	}

	registry := cmd.NewRegistry(log, st, cfg.Prefix)
	registry.Add(core.Commands()...)
	registry.AddMenus(core.UserInfoMenu())

	nsb := notsobot.New(imagefind.New(cfg.TenorAPIKey), magick)
	nsbCommands := nsb.Commands()
	registry.Add(nsbCommands...)
	registry.AddMenus(nsb.Menus(nsbCommands)...)
	registry.Add(moderation.Commands()...)
	searcher := search.New(cfg.GeniusToken)
	registry.Add(searcher.Commands()...)
	for prefix, h := range searcher.Components() {
		registry.HandleComponent(prefix, h)
	}
	fetcher := rss.NewFetcher()
	// The scheduler's REST client is set once the Discord client exists.
	scheduler := reminders.NewScheduler(st, nil, log)
	registry.Add(reminders.New(scheduler).Commands()...)
	registry.Add(feeds.New(fetcher).Commands()...)
	if err := registry.Validate(); err != nil {
		return err
	}

	client, err := disgo.New(cfg.Token,
		bot.WithLogger(log),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
				gateway.IntentMessageContent,
			),
			gateway.WithPresenceOpts(gateway.WithListeningActivity("/help")),
		),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagGuilds, cache.FlagRoles, cache.FlagChannels, cache.FlagMembers)),
		bot.WithEventListeners(registry),
	)
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	if err := registry.Sync(client, cfg.DevGuildIDs); err != nil {
		return err
	}
	log.Info("synced application commands", "guilds", cfg.DevGuildIDs)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.OpenGateway(ctx); err != nil {
		return err
	}
	log.Info("bot is running", "prefix", cfg.Prefix)

	pollCtx, stopPolling := context.WithCancel(context.Background())
	defer stopPolling()
	poller := &rss.Poller{Store: st, Fetcher: fetcher, Poster: client.Rest, Interval: cfg.RSSInterval, Log: log}
	go poller.Run(pollCtx)
	scheduler.Rest = client.Rest
	go scheduler.Run(pollCtx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
	return nil
}
