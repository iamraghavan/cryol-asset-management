package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"golang.org/x/sys/unix"
)

func collectPlatform(ctx context.Context, snapshot *inventory.Snapshot) {
	snapshot.Manufacturer = "Apple"
	snapshot.Model, _ = unix.Sysctl("hw.model")
	snapshot.CPUModel, _ = unix.Sysctl("machdep.cpu.brand_string")
	snapshot.MemoryBytes, _ = unix.SysctlUint64("hw.memsize")
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	snapshot.OSName = macOSProductName(ctx)
	// Package receipts do not reliably represent all currently installed apps.
	// Leave software unavailable until an authoritative collector is implemented.
}

func macOSProductName(ctx context.Context) string {
	raw, err := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion").Output()
	if err != nil {
		return "macOS"
	}
	version := strings.TrimSpace(string(raw))
	if version == "" || len(version) > 32 {
		return "macOS"
	}
	for _, segment := range strings.Split(version, ".") {
		if _, err := strconv.ParseUint(segment, 10, 16); err != nil {
			return "macOS"
		}
	}
	return "macOS " + version
}
