package main

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestVerificationGateAndProof(t *testing.T) {
	b, api := testBot(t)
	key := strings.Repeat("00", 32)
	config, err := newVerificationConfig("https://verify.example.com/", key)
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
	nonce, expiresAt, _, err := b.store.challenge(2, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(b.verify.challengeURL(2, expiresAt, nonce))
	if err != nil || u.Query().Get("challenge") == "" {
		t.Fatal("challenge URL missing")
	}
	proof := b.verify.ticket("p", 2, time.Now().Unix()+300, nonce)
	if _, valid := b.verify.parseProof(proof, 3, time.Now().Unix()); valid {
		t.Fatal("proof accepted for wrong user")
	}
	if _, valid := b.verify.parseProof(proof+"x", 2, time.Now().Unix()); valid {
		t.Fatal("tampered proof accepted")
	}
	service := privateMessage(2, 13, "")
	service.WebAppData = &WebAppData{Data: proof}
	if err := b.handle(ctx, service, 101); err != nil {
		t.Fatal(err)
	}
	verified, err := b.store.isVerified(2)
	if err != nil || !verified {
		t.Fatal("proof did not verify user", err)
	}
	if err := b.handle(ctx, m, 102); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatal("verified message not forwarded")
	}
	if accepted, err := b.store.consumeProof(103, 2, nonce, time.Now().Unix()); err != nil || accepted {
		t.Fatal("proof replay accepted", err)
	}
}

func TestVerificationConfig(t *testing.T) {
	if config, err := newVerificationConfig("", ""); err != nil || config != nil {
		t.Fatal("verification should be off by default")
	}
	for _, pair := range [][2]string{{"https://example.com", ""}, {"http://example.com", strings.Repeat("00", 32)}, {"https://example.com", "bad"}} {
		if _, err := newVerificationConfig(pair[0], pair[1]); err == nil {
			t.Fatal("invalid config accepted", pair)
		}
	}
}
