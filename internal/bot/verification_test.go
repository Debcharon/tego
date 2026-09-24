package bot

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Debcharon/tego/internal/telegram"
	"github.com/Debcharon/tego/internal/verification"
)

func TestVerificationGateAndProof(t *testing.T) {
	b, api := testBot(t)
	key := strings.Repeat("00", 32)
	config, err := verification.New("https://verify.example.com/", key)
	if err != nil {
		t.Fatal(err)
	}
	b.verify = config
	ctx := context.Background()
	m := privateMessage(2, 12, "private")
	if err := b.handle(ctx, m, 100); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 0 || len(api.sent) != 1 {
		t.Fatal("unverified message was forwarded or prompt missing")
	}
	nonce, expiresAt, _, err := b.store.Challenge(2, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(b.verify.ChallengeURL(2, expiresAt, nonce))
	if err != nil || u.Query().Get("challenge") == "" {
		t.Fatal("challenge URL missing")
	}
	proof := b.verify.Ticket("p", 2, time.Now().Unix()+300, nonce)
	if _, valid := b.verify.ParseProof(proof, 3, time.Now().Unix()); valid {
		t.Fatal("proof accepted for wrong user")
	}
	if _, valid := b.verify.ParseProof(proof+"x", 2, time.Now().Unix()); valid {
		t.Fatal("tampered proof accepted")
	}
	service := privateMessage(2, 13, "")
	service.WebAppData = &telegram.WebAppData{Data: proof}
	if err := b.handle(ctx, service, 101); err != nil {
		t.Fatal(err)
	}
	verified, err := b.store.IsVerified(2)
	if err != nil || !verified {
		t.Fatal("proof did not verify user", err)
	}
	if err := b.handle(ctx, m, 102); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatal("verified message not forwarded")
	}
	if accepted, err := b.store.ConsumeProof(103, 2, nonce, time.Now().Unix()); err != nil || accepted {
		t.Fatal("proof replay accepted", err)
	}
}
