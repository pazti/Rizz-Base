package core

import "testing"

func FuzzDecodeProject(f *testing.F) {
	f.Add([]byte(`{"version":1,"name":"test","createdAt":"2025-01-01T00:00:00Z","lobbies":{}}`))
	f.Add([]byte(`{"version":1,"name":"test","createdAt":"bad","lobbies":{}}`))
	f.Add([]byte(`{} {}`))
	f.Add([]byte{0, 1, 2, 255})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = decodeProject(data)
	})
}
