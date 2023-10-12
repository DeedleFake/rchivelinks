package main

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	is "github.com/wabarc/archive.is"
)

type Archiver struct {
	Archiver *is.Archiver

	once    sync.Once
	results chan Result
	errors  chan error
}

func (a *Archiver) init() {
	a.once.Do(func() {
		a.results = make(chan Result)
		a.errors = make(chan error)
	})
}

func (a *Archiver) result(ctx context.Context, link, result string) {
	select {
	case <-ctx.Done():
	case a.results <- Result{link, result}:
	}
}

func (a *Archiver) error(ctx context.Context, err error) {
	select {
	case <-ctx.Done():
	case a.errors <- err:
	}
}

func (a *Archiver) Results() <-chan Result {
	a.init()
	return a.results
}

func (a *Archiver) Errors() <-chan error {
	a.init()
	return a.errors
}

func (a *Archiver) Archive(ctx context.Context, link string) {
	go a.archive(ctx, link)
}

func (a *Archiver) archive(ctx context.Context, link string) {
	a.init()

	u, err := url.Parse(link)
	if err != nil {
		a.error(ctx, fmt.Errorf("%v -> parse: %w", link, err))
		return
	}

	result, err := a.Archiver.Wayback(ctx, u)
	if err != nil {
		a.error(ctx, fmt.Errorf("%v -> archive: %w", link, err))
		return
	}

	a.result(ctx, link, result)
}

type Result struct {
	Link, Result string
}
