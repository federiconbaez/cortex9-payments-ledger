package httpapi

import "testing"

func FuzzDecodeOpen(f *testing.F) {
	f.Add([]byte(`{"id":"a","shard":"s1","currency":"ARS","credit_limit":0}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"id":`))
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = DecodeOpen(body)
	})
}

func FuzzDecodeTransfer(f *testing.F) {
	f.Add([]byte(`{"from":"a","to":"b","amount":10,"currency":"USD"}`))
	f.Add([]byte(`{"from":"a","to":"a","amount":-1,"currency":"xx"}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = DecodeTransfer(body)
	})
}
