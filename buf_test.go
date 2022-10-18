package ringbuffer

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestRingBuffer(t *testing.T) {
	smallbuf := make([]byte, 5)
	copy(smallbuf, "12345")
	r := New(true)
	t.Log(r.Write(smallbuf))
	copy(smallbuf, "23456")
	t.Log(r.Read(smallbuf))
	t.Log(string(smallbuf))

	bigbuf := make([]byte, 4097)
	bigbuf1 := make([]byte, 4097)
	io.ReadFull(rand.Reader, bigbuf)
	t.Log(bigbuf[4094], bigbuf[4095], bigbuf[4096])
	t.Log(r.Write(bigbuf))
	t.Log(r.Read(bigbuf1))
	t.Log(bigbuf1[4094], bigbuf1[4095], bigbuf1[4096])
	// 1MB
	hugebuf := make([]byte, 1e6)
	hugebuf1 := make([]byte, 1e6)
	io.ReadFull(rand.Reader, hugebuf)
	t.Log(r.Write(hugebuf))
	t.Log(r.Read(hugebuf1))
	t.Log(bytes.Equal(hugebuf, hugebuf1))

}
func BenchmarkRingBuffer(b *testing.B) {
	hugebuf := make([]byte, 10e6)
	r := New(false)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		io.ReadFull(rand.Reader, hugebuf)
		r.Write(hugebuf)
		r.Read(hugebuf)
	}

}
