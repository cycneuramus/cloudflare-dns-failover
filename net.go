package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
)

func createAPIClient(apiKey string) (*cf.API, error) {
	api, err := cf.NewWithAPIToken(apiKey, cf.HTTPClient(&http.Client{Timeout: 15 * time.Second}))
	if err != nil {
		return api, fmt.Errorf("Error creating Cloudflare client: %v", err)
	}

	return api, nil
}

func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Do not follow redirects
		},
	}
}

func isHTTPResponsive(httpClient *http.Client, ip string) (bool, error) {
	resp, err := httpClient.Get("http://" + ip)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	return true, nil
}

func getResponsiveIP(httpClient *http.Client, rec Record) (string, error) {
	var failures []string

	for _, ip := range rec.IPs {
		ok, err := isHTTPResponsive(httpClient, ip)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}

		if ok {
			return ip, nil
		}
	}

	return "", fmt.Errorf("%s: No responsive IPs found", strings.Join(failures, "; "))
}
