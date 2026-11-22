package main

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"golang.org/x/sys/windows/registry"
)

func value(key registry.Key, name string) string {
	text, _, _ := key.GetStringValue(name)
	return strings.TrimSpace(text)
}

func collectPlatform(ctx context.Context, s *inventory.Snapshot) {
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		s.OSName = strings.TrimSpace(value(key, "ProductName") + " " + value(key, "DisplayVersion") + " " + value(key, "CurrentBuild"))
		key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		s.Manufacturer = value(key, "SystemManufacturer")
		s.Model = value(key, "SystemProductName")
		key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s.CPUModel = value(key, "ProcessorNameString")
		key.Close()
	}

	const uninstall = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	var records []inventory.Software
	for _, view := range []struct {
		flag uint32
		name string
	}{{registry.WOW64_64KEY, "registry64"}, {registry.WOW64_32KEY, "registry32"}} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstall, registry.ENUMERATE_SUB_KEYS|view.flag)
		if err == registry.ErrNotExist {
			continue
		}
		if err != nil {
			return
		}
		names, err := key.ReadSubKeyNames(20001)
		key.Close()
		if (err != nil && !errors.Is(err, io.EOF)) || len(names) > 20000 {
			return
		}
		for _, name := range names {
			if ctx.Err() != nil {
				return
			}
			sub, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstall+`\`+name, registry.QUERY_VALUE|view.flag)
			if err != nil {
				return
			}
			label := value(sub, "DisplayName")
			if label != "" {
				records = append(records, inventory.Software{ID: view.name + ":" + name, Name: label, Version: value(sub, "DisplayVersion"), Publisher: value(sub, "Publisher"), Source: "registry"})
			}
			sub.Close()
		}
	}
	s.Software = records
	s.SoftwareStatus = "complete"
}
