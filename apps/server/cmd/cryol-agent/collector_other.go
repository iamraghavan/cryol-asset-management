//go:build !linux && !windows && !darwin

package main

import (
	"context"
	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
)

func collectPlatform(_ context.Context, _ *inventory.Snapshot) {}
