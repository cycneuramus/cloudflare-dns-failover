package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpClient := createHTTPClient()
	api, err := createAPIClient(cfg.APIKey)
	if err != nil {
		log.Fatal(err)
	}

	states := make([]*DNSState, len(cfg.Records))
	for idx, rec := range cfg.Records {
		states[idx], err = fetchInitialState(ctx, api, rec)
		if err != nil {
			log.Fatal(err)
		}
	}

	interval := time.Duration(cfg.CheckInterval) * time.Second

	var wg sync.WaitGroup
	for idx, rec := range cfg.Records {
		wg.Add(1)
		go func(rec Record, state *DNSState) {
			defer wg.Done()
			monitor(ctx, api, httpClient, rec, state, interval)
		}(rec, states[idx])
	}

	<-ctx.Done()
	wg.Wait()
}
