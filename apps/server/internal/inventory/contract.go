package inventory

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxReportBytes = 4 << 20

type Software struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Publisher    string `json:"publisher,omitempty"`
	Source       string `json:"source"`
	Architecture string `json:"architecture,omitempty"`
}

type Adapter struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac,omitempty"`
	Up        bool     `json:"up"`
	Addresses []string `json:"addresses,omitempty"`
}

type Volume struct {
	Mount         string `json:"mount"`
	CapacityBytes uint64 `json:"capacityBytes"`
	FreeBytes     uint64 `json:"freeBytes"`
}

type Snapshot struct {
	FirmwareVendor  string     `json:"firmwareVendor,omitempty"`
	FirmwareVersion string     `json:"firmwareVersion,omitempty"`
	Volumes         []Volume   `json:"volumes,omitempty"`
	Version         int        `json:"version"`
	RunID           string     `json:"runId"`
	ScannedAt       time.Time  `json:"scannedAt"`
	AgentVersion    string     `json:"agentVersion"`
	Hostname        string     `json:"hostname"`
	OSFamily        string     `json:"osFamily"`
	OSName          string     `json:"osName"`
	Architecture    string     `json:"architecture"`
	LogicalCPUs     int        `json:"logicalCpus"`
	CPUModel        string     `json:"cpuModel,omitempty"`
	MemoryBytes     uint64     `json:"memoryBytes,omitempty"`
	Manufacturer    string     `json:"manufacturer,omitempty"`
	Model           string     `json:"model,omitempty"`
	Serial          string     `json:"serial,omitempty"`
	SystemUUID      string     `json:"systemUuid,omitempty"`
	Network         []Adapter  `json:"network,omitempty"`
	Software        []Software `json:"software,omitempty"`
	SoftwareStatus  string     `json:"softwareStatus"`
}

func NewID() string {
	var id [16]byte
	rand.Read(id[:])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return hex.EncodeToString(id[:4]) + "-" + hex.EncodeToString(id[4:6]) + "-" + hex.EncodeToString(id[6:8]) + "-" + hex.EncodeToString(id[8:10]) + "-" + hex.EncodeToString(id[10:])
}

func textOK(s string, max int) bool {
	return utf8.ValidString(s) && len(s) <= max && !strings.ContainsRune(s, 0)
}

func (s Snapshot) Validate(now time.Time) error {
	if s.Version != 1 || s.ScannedAt.IsZero() || s.ScannedAt.After(now.Add(10*time.Minute)) || s.ScannedAt.Before(now.Add(-30*24*time.Hour)) ||
		(s.OSFamily != "windows" && s.OSFamily != "linux") || s.Hostname == "" || s.AgentVersion == "" ||
		s.LogicalCPUs < 1 || s.LogicalCPUs > 65536 ||
		(s.SoftwareStatus != "complete" && s.SoftwareStatus != "unavailable") || len(s.Network) > 256 || len(s.Software) > 20000 {
		return errors.New("unsupported or out-of-bounds inventory")
	}
	for _, value := range []string{s.Hostname, s.AgentVersion, s.OSName, s.Architecture, s.CPUModel, s.Manufacturer, s.Model, s.Serial, s.SystemUUID, s.FirmwareVendor, s.FirmwareVersion} {
		if !textOK(value, 512) {
			return errors.New("invalid device field")
		}
	}
	if len(s.Volumes) > 128 {
		return errors.New("too many volumes")
	}
	for _, volume := range s.Volumes {
		if volume.Mount == "" || !textOK(volume.Mount, 256) || volume.CapacityBytes == 0 || volume.FreeBytes > volume.CapacityBytes {
			return errors.New("invalid volume data")
		}
	}
	if s.SoftwareStatus == "unavailable" && len(s.Software) != 0 {
		return errors.New("unavailable software collector returned records")
	}
	for _, adapter := range s.Network {
		if !textOK(adapter.Name, 256) || !textOK(adapter.MAC, 64) || len(adapter.Addresses) > 128 {
			return errors.New("invalid network adapter")
		}
		for _, address := range adapter.Addresses {
			if _, err := netip.ParsePrefix(address); err != nil {
				return errors.New("invalid network address")
			}
		}
	}
	seen := make(map[string]bool, len(s.Software))
	for _, product := range s.Software {
		key := product.Source + "\x00" + product.ID
		if seen[key] || product.ID == "" || product.Name == "" || (product.Source != "dpkg" && product.Source != "rpm" && product.Source != "registry") {
			return errors.New("invalid or duplicate software identity")
		}
		seen[key] = true
		for _, value := range []string{product.ID, product.Name, product.Publisher, product.Version, product.Architecture} {
			if !textOK(value, 512) {
				return errors.New("invalid software field")
			}
		}
	}
	return nil
}

func IsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}
