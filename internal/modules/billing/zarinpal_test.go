package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeZarinpal answers like ZarinPal's v4 API; reply decides each response.
func fakeZarinpal(t *testing.T, reply func(path string, body map[string]any) (int, string)) (zarinpalProvider, *[]map[string]any) {
	t.Helper()
	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, body)
		code, out := reply(r.URL.Path, body)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	p := newZarinpal("00000000-0000-0000-0000-000000000000", false)
	p.apiBase, p.startPay = srv.URL+"/pg/v4/payment", srv.URL+"/pg/StartPay/"
	return p, &seen
}

func TestZarinpalRequestAndVerify(t *testing.T) {
	ctx := context.Background()
	verifyCode := 100
	p, seen := fakeZarinpal(t, func(path string, _ map[string]any) (int, string) {
		if strings.HasSuffix(path, "/request.json") {
			return 200, `{"data":{"code":100,"message":"Success","authority":"A0000000000000000000000000000abcd","fee_type":"Merchant","fee":100},"errors":[]}`
		}
		return 200, `{"data":{"code":` + map[bool]string{true: "100", false: "101"}[verifyCode == 100] + `,"message":"Verified","ref_id":201,"card_pan":"502229******5995"},"errors":[]}`
	})
	auth, url, err := p.RequestPayment(ctx, 99000, "Leksuu monthly", "https://leksuu.ir/api/v1/billing/callback")
	if err != nil || auth != "A0000000000000000000000000000abcd" || !strings.HasSuffix(url, "/pg/StartPay/"+auth) {
		t.Fatalf("request: auth=%q url=%q err=%v", auth, url, err)
	}
	if amt := (*seen)[0]["amount"].(float64); amt != 990000 {
		t.Fatalf("amount must be sent in Rial (Toman×10): got %v", amt)
	}
	if ref, err := p.VerifyPayment(ctx, auth, 99000); err != nil || ref != "201" {
		t.Fatalf("verify 100: ref=%q err=%v", ref, err)
	}
	verifyCode = 101 // the same payment verified again
	if ref, err := p.VerifyPayment(ctx, auth, 99000); err != nil || ref != "201" {
		t.Fatalf("verify 101 (already verified) must succeed: ref=%q err=%v", ref, err)
	}
}

// ZarinPal reports failures with `data: []` and `errors` as an object or a
// list; the adapter must surface ZarinPal's own code, not a decode error.
func TestZarinpalErrorsAreReported(t *testing.T) {
	ctx := context.Background()
	for name, body := range map[string]string{
		"errors object": `{"data":[],"errors":{"code":-9,"message":"The input params invalid, validation error.","validations":[]}}`,
		"errors list":   `{"data":[],"errors":[{"code":-11,"message":"Merchant is not active"}]}`,
	} {
		p, _ := fakeZarinpal(t, func(string, map[string]any) (int, string) { return 422, body })
		_, _, err := p.RequestPayment(ctx, 1000, "x", "https://cb")
		if err == nil || !strings.Contains(err.Error(), "zarinpal error -") {
			t.Fatalf("%s: want ZarinPal's error code, got %v", name, err)
		}
	}
	// A verify that ZarinPal rejects (e.g. -51 failed payment) is a failure.
	p, _ := fakeZarinpal(t, func(string, map[string]any) (int, string) {
		return 200, `{"data":[],"errors":{"code":-51,"message":"Session is not valid, session is not active paid try."}}`
	})
	if _, err := p.VerifyPayment(ctx, "A1", 1000); err == nil || !strings.Contains(err.Error(), "-51") {
		t.Fatalf("verify failure: %v", err)
	}
}

func TestZarinpalHosts(t *testing.T) {
	if p := newZarinpal("m", false); p.apiBase != "https://payment.zarinpal.com/pg/v4/payment" || p.startPay != "https://payment.zarinpal.com/pg/StartPay/" {
		t.Fatalf("production hosts: %+v", p)
	}
	if p := newZarinpal("m", true); p.apiBase != "https://sandbox.zarinpal.com/pg/v4/payment" || p.startPay != "https://sandbox.zarinpal.com/pg/StartPay/" {
		t.Fatalf("sandbox hosts: %+v", p)
	}
}
