package config

import "testing"

// With ZarinPal, production refuses localhost/non-https return addresses and
// a missing merchant id (paying users must come back to the real site).
func TestZarinpalConfigIsValidated(t *testing.T) {
	good := Config{Env: "production", JWTSecret: "a-very-long-random-secret-value-1234567890",
		PaymentProvider: "zarinpal", ZarinpalMerchantID: "11111111-2222-3333-4444-555555555555",
		BillingCallbackURL: "https://leksuu.ir/api/v1/billing/callback",
		BillingSuccessURL:  "https://leksuu.ir/fa/billing/success", BillingFailURL: "https://leksuu.ir/fa/billing/fail"}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	bad := good
	bad.BillingCallbackURL = "http://localhost:8080/api/v1/billing/callback"
	if bad.Validate() == nil {
		t.Fatal("localhost callback must be refused")
	}
	bad = good
	bad.ZarinpalMerchantID = ""
	if bad.Validate() == nil {
		t.Fatal("missing merchant id must be refused")
	}
}
