package sub

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestClientRemarksExcludeInboundName(t *testing.T) {
	inbound := &model.Inbound{Remark: "PRIVATE-INBOUND"}
	for _, pattern := range []string{"-ieo", "_eio", "-i", "", "-eo"} {
		s := NewSubService(false, pattern)
		got := s.genRemark(inbound, "client-name", "")
		if got != "client-name" {
			t.Fatalf("%q: %q", pattern, got)
		}
		got = s.genRemark(inbound, "client-name", "extra")
		if strings.Contains(got, inbound.Remark) {
			t.Fatal("inbound name leaked", got)
		}
	}
}
