package command

import "testing"

// BenchmarkSet and BenchmarkGet measure the command path for the two hottest
// operations against a fixed payload. They report ns/op and allocs/op; the
// project makes no throughput claims, the numbers are here to catch regressions.

func BenchmarkSet(b *testing.B) {
	r, ctx, _ := newTestEnvN(1)
	args := [][]byte{[]byte("SET"), []byte("key"), []byte("a fixed 32 byte benchmark payload")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Dispatch(ctx, args)
	}
}

func BenchmarkGet(b *testing.B) {
	r, ctx, _ := newTestEnvN(1)
	r.Dispatch(ctx, [][]byte{[]byte("SET"), []byte("key"), []byte("a fixed 32 byte benchmark payload")})
	args := [][]byte{[]byte("GET"), []byte("key")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Dispatch(ctx, args)
	}
}

// BenchmarkResolve measures command lookup and arity validation in isolation.
// It runs on every command, so it should not allocate.
func BenchmarkResolve(b *testing.B) {
	r := NewRegistry()
	args := [][]byte{[]byte("GET"), []byte("key")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := r.Resolve(args); !ok {
			b.Fatal("resolve failed")
		}
	}
}
