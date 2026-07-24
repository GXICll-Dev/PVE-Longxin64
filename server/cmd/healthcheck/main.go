package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const probeTimeout = 3 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: pve-classroom-healthcheck <url>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return probe(ctx, &http.Client{Timeout: probeTimeout}, args[0])
}

func probe(ctx context.Context, client *http.Client, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("create healthcheck request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("healthcheck returned %s", response.Status)
	}
	return nil
}
