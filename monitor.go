package main

import (
	"context"
	"log"
	"net/http"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
)

func check(ctx context.Context, api *cf.API, httpClient *http.Client, rec Record, state *DNSState) string {
	ip, err := getResponsiveIP(httpClient, rec)
	if err != nil {
		return err.Error()
	}

	msg, err := ensureIP(ctx, api, rec, state, ip)
	if err != nil {
		return err.Error()
	}

	return msg
}

func monitor(ctx context.Context, api *cf.API, httpClient *http.Client, rec Record, state *DNSState, interval time.Duration) {
	lastMsg := ""

	for {
		msg := check(ctx, api, httpClient, rec, state)
		if msg != lastMsg {
			log.Printf("%s: %s", rec.Domain, msg)
			lastMsg = msg
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
