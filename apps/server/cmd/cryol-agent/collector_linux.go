package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
)

func readInfo(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
}

func collectPlatform(ctx context.Context, s *inventory.Snapshot) {
	for _, line := range strings.Split(readInfo("/etc/os-release"), "\n") {
		if val, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			s.OSName = strings.Trim(val, `"'`)
			break
		}
	}
	for _, line := range strings.Split(readInfo("/proc/cpuinfo"), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "model name" {
			s.CPUModel = strings.TrimSpace(val)
			break
		}
	}
	for _, line := range strings.Split(readInfo("/proc/meminfo"), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[2] == "kB" {
				if total, err := strconv.ParseUint(fields[1], 10, 54); err == nil {
					s.MemoryBytes = total * 1024
				}
			}
			break
		}
	}
	s.Manufacturer = readInfo("/sys/class/dmi/id/sys_vendor")
	s.Model = readInfo("/sys/class/dmi/id/product_name")
	s.Serial = readInfo("/sys/class/dmi/id/product_serial")
	s.SystemUUID = readInfo("/sys/class/dmi/id/product_uuid")

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	manager := "dpkg"
	command := "/usr/bin/dpkg-query"
	args := []string{"-W", "-f=${db:Status-Abbrev}\t${binary:Package}\t${Package}\t${Version}\t${Architecture}\n"}
	if _, err := os.Stat(command); os.IsNotExist(err) {
		manager = "rpm"
		command = "/usr/bin/rpm"
		args = []string{"-qa", "--qf", "%{NAME}\t%{EPOCHNUM}:%{VERSION}-%{RELEASE}\t%{ARCH}\n"}
	}
	cmd := exec.CommandContext(ctx, command, args...)
	output := &limitedOutput{}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return
	}
	seen := map[string]bool{}
	var records []inventory.Software
	for _, line := range strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		var product inventory.Software
		if manager == "dpkg" {
			if len(fields) != 5 {
				return
			}
			if !strings.HasPrefix(fields[0], "ii") {
				continue
			}
			product = inventory.Software{ID: fields[1], Name: fields[2], Version: fields[3], Architecture: fields[4], Source: manager}
		} else {
			if len(fields) != 3 {
				return
			}
			product = inventory.Software{ID: fields[0] + ":" + fields[2] + ":" + fields[1], Name: fields[0], Version: fields[1], Architecture: fields[2], Source: manager}
		}
		if !seen[product.ID] {
			records = append(records, product)
			seen[product.ID] = true
		}
	}
	s.Software = records
	s.SoftwareStatus = "complete"
}

type limitedOutput struct{ bytes.Buffer }

func (out *limitedOutput) Write(data []byte) (int, error) {
	if out.Len()+len(data) > inventory.MaxReportBytes {
		return 0, os.ErrInvalid
	}
	return out.Buffer.Write(data)
}
