package xray

import "testing"

func TestClientSourceRecordExcludesDNSTTAndKeepsDirectIPs(t *testing.T) {
	for _, source := range []string{"slowdns|fd53:1:2:3::4", "fd53:1:2:3::4", "[FD53:1:2:3::4]"} {
		if _, _, ok := clientSourceRecord(source, 123); ok {
			t.Fatalf("DNSTT entered IP snapshot: %s", source)
		}
	}
	for _, source := range []string{"103.31.152.238", "::ffff:103.31.152.238"} {
		record, key, ok := clientSourceRecord(source, 123)
		if !ok || key != "103.31.152.238" || record.IP != key || record.Timestamp != 123 {
			t.Fatalf("ordinary IP was changed: %+v %s %t", record, key, ok)
		}
	}
}
