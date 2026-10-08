package billing

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/leksuu/platform/internal/modules/billing/usecase"
)

// stubPaymentProvider simulates a gateway without any network. RequestPayment
// returns a pay URL that points straight back at our own callback with Status=OK,
// so the whole subscribe -> callback -> activate flow works locally with no gateway.
type stubPaymentProvider struct{}

func (stubPaymentProvider) Name() string { return "stub" }

func (stubPaymentProvider) RequestPayment(_ context.Context, _ int, _ string, callbackURL string) (string, string, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	authority := "STUB-" + hex.EncodeToString(b)
	sep := "?"
	if u, err := neturl.Parse(callbackURL); err == nil && u.RawQuery != "" {
		sep = "&"
	}
	payURL := callbackURL + sep + "Authority=" + neturl.QueryEscape(authority) + "&Status=OK"
	return authority, payURL, nil
}

func (stubPaymentProvider) VerifyPayment(_ context.Context, authority string, _ int) (string, error) {
	return "STUBREF-" + authority, nil
}

// disabledPaymentProvider replaces the stub where simulated payments must not be
// served (production) and stands in for a gateway that is not configured: the
// stub approves every payment without charging anything, so offering it to real
// users would hand out subscriptions for free. Every purchase fails with
// "payments unavailable" until a real gateway is configured.
type disabledPaymentProvider struct{}

func (disabledPaymentProvider) Name() string { return "disabled" }

func (disabledPaymentProvider) RequestPayment(context.Context, int, string, string) (string, string, error) {
	return "", "", usecase.ErrPaymentsUnavailable
}

func (disabledPaymentProvider) VerifyPayment(context.Context, string, int) (string, error) {
	return "", usecase.ErrPaymentsUnavailable
}

// zarinpalProvider talks to ZarinPal's v4 REST API (Iran's most common gateway).
// Amounts are sent in Rial (Toman * 10). NOTE: exercised only against the live
// gateway; not run in CI. Behind the same port as the stub, so nothing else changes.
type zarinpalProvider struct {
	merchantID string
	client     *http.Client
	startPay   string // base of the StartPay redirect
	apiBase    string
}

func newZarinpal(merchantID string) zarinpalProvider {
	return zarinpalProvider{
		merchantID: merchantID,
		client:     &http.Client{Timeout: 20 * time.Second},
		startPay:   "https://www.zarinpal.com/pg/StartPay/",
		apiBase:    "https://api.zarinpal.com/pg/v4/payment",
	}
}

func (zarinpalProvider) Name() string { return "zarinpal" }

func (p zarinpalProvider) RequestPayment(ctx context.Context, amountToman int, desc, callbackURL string) (string, string, error) {
	body := map[string]any{
		"merchant_id":  p.merchantID,
		"amount":       amountToman * 10, // Rial
		"description":  desc,
		"callback_url": callbackURL,
	}
	var out struct {
		Data struct {
			Authority string `json:"authority"`
			Code      int    `json:"code"`
		} `json:"data"`
	}
	if err := p.post(ctx, p.apiBase+"/request.json", body, &out); err != nil {
		return "", "", err
	}
	if out.Data.Authority == "" {
		return "", "", fmt.Errorf("zarinpal: empty authority")
	}
	return out.Data.Authority, p.startPay + out.Data.Authority, nil
}

func (p zarinpalProvider) VerifyPayment(ctx context.Context, authority string, amountToman int) (string, error) {
	body := map[string]any{
		"merchant_id": p.merchantID,
		"amount":      amountToman * 10,
		"authority":   authority,
	}
	var out struct {
		Data struct {
			RefID int64 `json:"ref_id"`
			Code  int   `json:"code"`
		} `json:"data"`
	}
	if err := p.post(ctx, p.apiBase+"/verify.json", body, &out); err != nil {
		return "", err
	}
	// 100 = verified, 101 = already verified
	if out.Data.Code != 100 && out.Data.Code != 101 {
		return "", fmt.Errorf("zarinpal: verify code %d", out.Data.Code)
	}
	return fmt.Sprintf("%d", out.Data.RefID), nil
}

func (p zarinpalProvider) post(ctx context.Context, url string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("zarinpal status %d", resp.StatusCode)
	}
	return json.Unmarshal(data, out)
}
