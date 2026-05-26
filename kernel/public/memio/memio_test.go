package memio_test

import (
	"context"
	"errors"
	"testing"
	"time"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

func TestBindingWithTxRejectsStaleConcurrentCommit(t *testing.T) {
	t.Parallel()

	binding := memio.NewBinding()
	started := make(chan struct{})
	release := make(chan struct{})
	errCh := make(chan error, 2)

	go func() {
		errCh <- binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
			if err := binding.PutToken(ctx, tx, token("tok_stale")); err != nil {
				return err
			}
			close(started)
			<-release
			return nil
		})
	}()

	<-started

	go func() {
		errCh <- binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
			return binding.PutToken(ctx, tx, token("tok_fresh"))
		})
	}()

	secondErr := <-errCh
	if secondErr != nil {
		t.Fatalf("fresh commit error = %v", secondErr)
	}

	close(release)

	firstErr := <-errCh
	if !errors.Is(firstErr, memio.ErrConcurrentWriteConflict) {
		t.Fatalf("stale commit error = %v, want %v", firstErr, memio.ErrConcurrentWriteConflict)
	}

	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		tokens, err := binding.Tokens(ctx, tx, "run_1")
		if err != nil {
			return err
		}
		if len(tokens) != 1 {
			t.Fatalf("stored tokens = %d, want 1", len(tokens))
		}
		if tokens[0].ID != "tok_fresh" {
			t.Fatalf("stored token id = %s, want tok_fresh", tokens[0].ID)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}

func token(id string) kernel.Token {
	return kernel.Token{
		ID:             id,
		RunID:          "run_1",
		TenantID:       "tenant_1",
		ValueCanonical: "1.000000000",
		CreatedAt:      time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
	}
}
