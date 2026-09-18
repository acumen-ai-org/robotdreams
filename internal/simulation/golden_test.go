package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestGeneratedHistoryIsStable(t *testing.T) {
	const want = "9340eeccae546df0574befcc69d4b3ef4e58ebce633462f54a195e09cb50f614"
	g := NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 48*time.Hour)
	blob, err := json.Marshal(g.History())
	if err != nil {
		t.Fatalf("marshal history: %v", err)
	}
	sum := sha256.Sum256(blob)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("history hash: got %s, want %s (%d bytes)", got, want, len(blob))
	}
}
