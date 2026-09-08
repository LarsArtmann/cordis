package loader

import (
	"bytes"
	"testing"
)

// FuzzConfigRoundtrip exercises the JSON config layer's invariants: every
// input decodes without panicking (errors are fine), every successful
// decode re-encodes, and encode/decode is idempotent - the encoded form of
// a decode is byte-stable under a second roundtrip.
func FuzzConfigRoundtrip(f *testing.F) {
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"plugins":[]}`))
	f.Add([]byte(`[{"id":"a","name":"timer","config":{"interval":1000}}]`))
	f.Add([]byte(`[{"name":"g","group":true,"disabled":false,"inject":{"a":1},"intercept":{"b":"x"},"isolate":{"c":[1,2]},"config":[1,"two",null,true,{"d":{}}]}]`))
	f.Add([]byte(`[{"name":"n","config":"scalar"},{"name":"m","config":1.5},{"name":"o","config":{"nested":{"deep":[{}]}}}]`))
	f.Add([]byte(`[{"name":"a","name":"b"}]`))
	f.Add([]byte(`{"plugins":null}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{not json`))

	f.Fuzz(func(t *testing.T, data []byte) {
		entries, err := DecodeConfig(data)
		if err != nil {
			return
		}
		encoded, err := EncodeConfig(entries)
		if err != nil {
			t.Fatalf("EncodeConfig failed on decoded entries: %v", err)
		}
		decoded, err := DecodeConfig(encoded)
		if err != nil {
			t.Fatalf("DecodeConfig failed on EncodeConfig output: %v", err)
		}
		reencoded, err := EncodeConfig(decoded)
		if err != nil {
			t.Fatalf("re-EncodeConfig failed: %v", err)
		}
		if !bytes.Equal(encoded, reencoded) {
			t.Fatalf("config roundtrip is byte-unstable:\nfirst:  %s\nsecond: %s", encoded, reencoded)
		}
	})
}
