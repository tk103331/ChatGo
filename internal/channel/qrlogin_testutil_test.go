package channel

import "testing"

// useTestWeComQR points the WeCom quick-setup endpoints at a test server.
func useTestWeComQR(t *testing.T, base string) {
	t.Helper()
	oldGen, oldQuery := wecomQRGenerateURL, wecomQRQueryURL
	wecomQRGenerateURL = base + "/ai/qc/generate"
	wecomQRQueryURL = base + "/ai/qc/query_result"
	t.Cleanup(func() {
		wecomQRGenerateURL, wecomQRQueryURL = oldGen, oldQuery
	})
}

// useTestDingTalkRegistration points the DingTalk device flow at a test server.
func useTestDingTalkRegistration(t *testing.T, base string) {
	t.Helper()
	old := dingtalkRegistrationBase
	dingtalkRegistrationBase = base + "/app/registration"
	t.Cleanup(func() { dingtalkRegistrationBase = old })
}
