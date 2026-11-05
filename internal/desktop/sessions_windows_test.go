package desktop

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
)

func TestProcessTableIncludesThisProcess(t *testing.T) {
	t.Parallel()

	table, err := processTable(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(table[os.Getppid()], os.Getpid()) {
		t.Fatalf("process %d missing under parent %d", os.Getpid(), os.Getppid())
	}
}

func TestProcessTableHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := processTable(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enumeration: %v", err)
	}
}

func TestWindowsOpenFilesIsExplicitlyUnsupported(t *testing.T) {
	t.Parallel()

	if _, err := openFiles(t.Context(), []int{os.Getpid()}); err == nil {
		t.Fatal("unsupported reader reported success")
	}
}
