//go:build !darwin || !cgo

package pdf

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPDFiumWaitCancellation(t *testing.T) {
	pool, err := pdfiumPool()
	if err != nil {
		t.Fatal(err)
	}
	instance, err := pool.GetInstanceWithContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = RenderCover(ctx, openFixture(t, renderFixture(0)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
