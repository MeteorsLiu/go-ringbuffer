package ringbuffer

import (
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

	hugebuf := make([]byte, 4097)
	hugebuf1 := make([]byte, 4097)
	io.ReadFull(rand.Reader, hugebuf)
	t.Log(hugebuf[4094], hugebuf[4095], hugebuf[4096])
	t.Log(r.Write(hugebuf))
	t.Log(r.Read(hugebuf1))
	t.Log(hugebuf1[4094], hugebuf1[4095], hugebuf1[4096])
}

func BenchmarkRingBuffer(b *testing.B) {
	hugebuf := make([]byte, 65536)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		io.ReadFull(rand.Reader, hugebuf)
		r.Write(hugebuf)
		r.Read(hugebuf)
	}

}
