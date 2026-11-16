package main

import (
	"context"
	"fmt"

	cf "github.com/cloudflare/cloudflare-go"
)

type DNSState struct {
	ActiveIP string
	ID       string
}

func fetchARecords(api *cf.API, ctx context.Context, rec Record) ([]cf.DNSRecord, error) {
	records, _, err := api.ListDNSRecords(ctx, cf.ZoneIdentifier(rec.ZoneID), cf.ListDNSRecordsParams{Name: rec.Domain, Type: "A"})
	return records, err
}

func fetchInitialState(ctx context.Context, api *cf.API, rec Record) (*DNSState, error) {
	records, err := fetchARecords(api, ctx, rec)
	if err != nil {
		return nil, fmt.Errorf("fetching DNS record for %s: %w", rec.Domain, err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no A record found for %s", rec.Domain)
	}

	return &DNSState{ActiveIP: records[0].Content, ID: records[0].ID}, nil
}

func updateARecord(ctx context.Context, api *cf.API, rec Record, state *DNSState, ip string) error {
	updateRecord := cf.UpdateDNSRecordParams{
		Type:    "A",
		Name:    rec.Domain,
		Content: ip,
		ID:      state.ID,
		TTL:     1,
		Proxied: &rec.Proxied,
	}

	_, err := api.UpdateDNSRecord(ctx, cf.ZoneIdentifier(rec.ZoneID), updateRecord)
	return err
}

func ensureIP(ctx context.Context, api *cf.API, rec Record, state *DNSState, ip string) (string, error) {
	if state.ActiveIP == ip {
		return fmt.Sprintf("already set to %s", ip), nil
	}

	err := updateARecord(ctx, api, rec, state, ip)
	if err != nil {
		return "", fmt.Errorf("DNS update failed: %w", err)
	}

	previous := state.ActiveIP
	state.ActiveIP = ip
	return fmt.Sprintf("updated to %s (previous: %s)", ip, previous), nil
}
