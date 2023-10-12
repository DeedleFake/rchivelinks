package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"

	is "github.com/wabarc/archive.is"
)

var urlRE = regexp.MustCompile(`\[([^]]+)\]()`)

func fixSourceURL(src string) (string, error) {
	p, err := url.Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if !strings.HasSuffix(p.Host, "reddit.com") {
		return "", errors.New("source URL not a Reddit URL")
	}

	parts := strings.Split(strings.TrimPrefix(p.Path, "/"), "/")
	if len(parts) < 4 {
		return "", errors.New("source URL not for a specific post")
	}
	if parts[2] != "comments" {
		return "", errors.New("source URL not for a post")
	}
	parts[3] = strings.TrimSuffix(parts[3], ".json") + ".json"

	return (&url.URL{
		Scheme: "https",
		Host:   "www.reddit.com",
		Path:   strings.Join(parts[:4], "/"),
	}).String(), nil
}

func getLinks(ctx context.Context, src string) ([]string, error) {
	src, err := fixSourceURL(src)
	if err != nil {
		return nil, fmt.Errorf("could not determine necessary info from source URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var data []struct {
		Data struct {
			Children []struct {
				Data struct {
					Selftext string `json:"selftext"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// TODO: Check for missing stuff instead of assuming its presence.
	matches := urlRE.FindAllStringSubmatch(data[0].Data.Children[0].Data.Selftext, -1)
	links := make([]string, 0, len(matches))
	for _, m := range matches {
		links = append(links, m[1])
	}
	return links, nil
}

func run(ctx context.Context) error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, filepath.Base(os.Args[0]))
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	src := flag.Arg(0)

	arc := Archiver{
		Archiver: is.NewArchiver(http.DefaultClient),
	}

	links, err := getLinks(ctx, src)
	if err != nil {
		return fmt.Errorf("get links from post: %w", err)
	}

	arc.Archive(ctx, src)
	for _, link := range links {
		arc.Archive(ctx, link)
	}

	for i := 0; i < len(links)+1; i++ {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case result := <-arc.Results():
			fmt.Printf("%v -> %v\n", result.Link, result.Result)
		case err := <-arc.Errors():
			fmt.Fprintln(os.Stderr, err)
		}
	}

	return nil
}

const usage = `Usage: %v <URL or ID>\n

rchivelinks is an automatic archiver for all links in a Reddit post as
well as the post itself. The primary intention is to produce an
archive of posts that are collecting information about something.

The URL passed should be a full URL for a Reddit post.`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
