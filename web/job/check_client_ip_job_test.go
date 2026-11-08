package job

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestMissingCoreSnapshotDoesNotReplaceRecords(t *testing.T) {
	setupIPTrackingDB(t)
	t.Setenv("XUI_BIN_FOLDER", filepath.Join(t.TempDir(), "missing"))
	var inbound service.InboundService
	if err := inbound.RefreshClientIPs(); err == nil {
		t.Fatal("missing core config reported as a successful empty snapshot")
	}
}

func TestLimitConfigWriteFailureIsReported(t *testing.T) {
	setupIPTrackingDB(t)
	path := filepath.Join(t.TempDir(), "file-not-directory")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_DB_FOLDER", path)
	if err := service.WriteClientIPLimits(); err == nil {
		t.Fatal("unwritable limits silently succeeded")
	}
}
