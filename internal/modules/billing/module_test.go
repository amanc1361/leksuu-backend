package billing

import "testing"

// The simulated stub approves payments without charging anything: outside
// development it must never be the active gateway.
func TestStubIsDisabledUnlessAllowed(t *testing.T) {
	cases := []struct {
		kind, merchant string
		allowStub      bool
		want           string
	}{
		{"stub", "", false, "disabled"},
		{"", "", false, "disabled"},
		{"zarinpal", "", false, "disabled"},
		{"stub", "", true, "stub"},
		{"zarinpal", "m-1", false, "zarinpal"},
	}
	for _, c := range cases {
		m := New(Deps{ProviderKind: c.kind, MerchantID: c.merchant, AllowStub: c.allowStub})
		if got := m.Service().ProviderName(); got != c.want {
			t.Errorf("%+v: provider %q, want %q", c, got, c.want)
		}
	}
}
