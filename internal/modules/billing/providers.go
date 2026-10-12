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

// zarinpalProvider talks to ZarinPal's v4 REST API (Iran's most common gateway):
// POST /pg/v4/payment/request.json → authority → the learner pays at
// /pg/StartPay/{authority} → ZarinPal redirects to our callback with
// Authority and Status=OK|NOK → POST /pg/v4/payment/verify.json settles it
// (code 100 the first time, 101 when already verified). Amounts are sent in
// Rial (Toman × 10). The sandbox accepts any UUID-shaped merchant id.
type zarinpalProvider struct {
	merchantID string
	client     *http.Client
	startPay   string // base of the StartPay redirect
	apiBase    string
}

const (
	zarinpalHost        = "https://payment.zarinpal.com"
	zarinpalSandboxHost = "https://sandbox.zarinpal.com"
)

func newZarinpal(merchantID string, sandbox bool) zarinpalProvider {
	host := zarinpalHost
	if sandbox {
		host = zarinpalSandboxHost
	}
	return zarinpalProvider{
		merchantID: merchantID,
		client:     &http.Client{Timeout: 20 * time.Second},
		startPay:   host + "/pg/StartPay/",
		apiBase:    host + "/pg/v4/payment",
	}
}

func (zarinpalProvider) Name() string { return "zarinpal" }

// zpResponse is ZarinPal's envelope. On failure `data` is an empty list and
// `errors` an object (sometimes a list), so both are decoded leniently.
type zpResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

type zpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errorOf returns ZarinPal's error, if the envelope carries one.
func (r zpResponse) errorOf() error {
	raw := bytes.TrimSpace(r.Errors)
	if len(raw) == 0 || string(raw) == "[]" || string(raw) == "{}" || string(raw) == "null" {
		return nil
	}
	var one zpError
	if json.Unmarshal(raw, &one) == nil && one.Code != 0 {
		return fmt.Errorf("zarinpal error %d: %s", one.Code, one.Message)
	}
	var many []zpError
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return fmt.Errorf("zarinpal error %d: %s", many[0].Code, many[0].Message)
	}
	return fmt.Errorf("zarinpal error: %s", raw)
}

func (p zarinpalProvider) RequestPayment(ctx context.Context, amountToman int, desc, callbackURL string) (string, string, error) {
	var data struct {
		Code      int    `json:"code"`
		Authority string `json:"authority"`
	}
	if err := p.call(ctx, "/request.json", map[string]any{
		"merchant_id":  p.merchantID,
		"amount":       amountToman * 10, // Rial
		"description":  desc,
		"callback_url": callbackURL,
	}, &data); err != nil {
		return "", "", err
	}
	if data.Code != 100 || data.Authority == "" {
		return "", "", fmt.Errorf("zarinpal: request code %d", data.Code)
	}
	return data.Authority, p.startPay + data.Authority, nil
}

func (p zarinpalProvider) VerifyPayment(ctx context.Context, authority string, amountToman int) (string, error) {
	var data struct {
		Code  int   `json:"code"`
		RefID int64 `json:"ref_id"`
	}
	if err := p.call(ctx, "/verify.json", map[string]any{
		"merchant_id": p.merchantID,
		"amount":      amountToman * 10,
		"authority":   authority,
	}, &data); err != nil {
		return "", err
	}
	// 100 = verified now, 101 = already verified (the callback came twice).
	if data.Code != 100 && data.Code != 101 {
		return "", fmt.Errorf("zarinpal: verify code %d", data.Code)
	}
	return fmt.Sprintf("%d", data.RefID), nil
}

// call posts to the API and decodes `data`, surfacing ZarinPal's own error.
func (p zarinpalProvider) call(ctx context.Context, path string, body any, data any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBase+path, bytes.NewReader(b))
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env zpResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("zarinpal: status %d, unreadable response", resp.StatusCode)
	}
	if err := env.errorOf(); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("zarinpal: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(env.Data, data); err != nil {
		return fmt.Errorf("zarinpal: unexpected data: %s", env.Data)
	}
	return nil
}
