package codemap

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/AniruthKarthik/codemap/internal/models"
	"github.com/AniruthKarthik/codemap/parser"
	"github.com/AniruthKarthik/codemap/scanner"
	"golang.org/x/sync/errgroup"
)

// RepositoryBuilder orchestrates the scanning and parsing of a repository.
type RepositoryBuilder struct {
	scanner scanner.Scanner
	parser  parser.Parser
	workers int
}

// BuilderOption defines a functional option for configuring a RepositoryBuilder.
type BuilderOption func(*RepositoryBuilder)

// WithScanner sets a custom scanner for the builder.
func WithScanner(s scanner.Scanner) BuilderOption {
	return func(b *RepositoryBuilder) {
		b.scanner = s
	}
}

// WithParser sets a custom parser for the builder.
func WithParser(p parser.Parser) BuilderOption {
	return func(b *RepositoryBuilder) {
		b.parser = p
	}
}

// WithWorkers sets the number of concurrent workers for parsing.
func WithWorkers(n int) BuilderOption {
	return func(b *RepositoryBuilder) {
		if n > 0 {
			b.workers = n
		}
	}
}

// NewRepositoryBuilder creates a new RepositoryBuilder with default configurations.
func NewRepositoryBuilder(opts ...BuilderOption) *RepositoryBuilder {
	b := &RepositoryBuilder{
		scanner: scanner.NewScanner(),
		parser:  parser.NewGoParser(),
		workers: runtime.NumCPU(),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Build walks the filesystem starting from root and builds a Repository model concurrently.
func (b *RepositoryBuilder) Build(ctx context.Context, root string) (*models.Repository, error) {
	paths, err := b.scanner.Scan(root)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	pathChan := make(chan string, len(paths))
	for _, path := range paths {
		pathChan <- path
	}
	close(pathChan)

	// We use a mutex to safely append to the results slice from multiple goroutines.
	// Alternatively, we could use a channel, but a mutex is efficient for simple appends.
	var mu sync.Mutex
	files := make([]*models.File, 0, len(paths))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(b.workers)

	for path := range pathChan {
		path := path // capture range variable
		g.Go(func() error {
			file, err := b.parser.Parse(path)
			if err != nil {
				return fmt.Errorf("failed to parse %q: %w", path, err)
			}

			mu.Lock()
			files = append(files, file)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &models.Repository{
		Files: files,
	}, nil
}

// BuildRepository is a convenience function that uses the default RepositoryBuilder.
func BuildRepository(root string) (*models.Repository, error) {
	builder := NewRepositoryBuilder()
	return builder.Build(context.Background(), root)
}
