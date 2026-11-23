package main

import (
	"testing"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"golang.org/x/sys/windows/registry"
)

func TestConfigureEventSourceRepairsExistingKey(t *testing.T) {
	path := `Software\CryolAgentTest-` + inventory.NewID()
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	if err := key.SetStringValue("EventMessageFile", `C:\Legacy\cryol-agent.exe`); err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue("Preserve", "existing data"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		created, err := configureEventSource(registry.CURRENT_USER, path)
		if err != nil || created {
			t.Fatalf("repair %d: created=%v, error=%v", i, created, err)
		}
		messageFile, valueType, err := key.GetStringValue("EventMessageFile")
		if err != nil || valueType != registry.EXPAND_SZ || messageFile != `%SystemRoot%\System32\EventCreate.exe` {
			t.Fatalf("unexpected message file registration: %q type=%d error=%v", messageFile, valueType, err)
		}
	}
	if value, _, err := key.GetStringValue("Preserve"); err != nil || value != "existing data" {
		t.Fatalf("unrelated value changed: %q (%v)", value, err)
	}
}
