package api

import "testing"

func TestName(t *testing.T) {
	addr, err := Name("namesrv.ns")
	t.Log(addr, err)
}

func BenchmarkName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		addr, err := Name("namesrv.ns")
		b.Log(addr, err)
	}
}

func BenchmarkNameParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			addr, err := Name("namesrv.ns")
			b.Log(addr, err)
		}
	})
}
