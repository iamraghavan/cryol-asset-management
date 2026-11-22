package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"unsafe"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"golang.org/x/sys/windows"
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
		s.FirmwareVendor = value(key, "BIOSVendor")
		s.FirmwareVersion = value(key, "BIOSVersion")
		key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s.CPUModel = value(key, "ProcessorNameString")
		key.Close()
	}

	s.MemoryBytes = totalPhysicalMemory()
	s.Volumes = fixedVolumes(ctx)

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

var memoryStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func totalPhysicalMemory() uint64 {
	var data struct {
		Length        uint32
		Load          uint32
		TotalPhysical uint64
		Remaining     [6]uint64
	}
	data.Length = uint32(unsafe.Sizeof(data))
	ok, _, _ := memoryStatus.Call(uintptr(unsafe.Pointer(&data)))
	if ok == 0 {
		return 0
	}
	return data.TotalPhysical
}

func fixedVolumes(ctx context.Context) []inventory.Volume {
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var volumes []inventory.Volume
	for index := uint(0); index < 26 && ctx.Err() == nil; index++ {
		if drives&(1<<index) == 0 {
			continue
		}
		mount := string(rune('A'+index)) + `:\`
		ptr, _ := windows.UTF16PtrFromString(mount)
		if windows.GetDriveType(ptr) != windows.DRIVE_FIXED {
			continue
		}
		var available, total, free uint64
		if windows.GetDiskFreeSpaceEx(ptr, &available, &total, &free) == nil && total > 0 && available <= total {
			volumes = append(volumes, inventory.Volume{Mount: mount, CapacityBytes: total, FreeBytes: available})
		}
	}
	return volumes
}
