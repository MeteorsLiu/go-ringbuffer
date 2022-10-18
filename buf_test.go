package ringbuffer

import (
	"crypto/rand"
	"encoding/hex"
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
	n, _ := io.ReadFull(rand.Reader, hugebuf)
	t.Log(hex.EncodeToString(hugebuf))
	t.Log(r.Write(hugebuf))
	t.Log(r.Read(hugebuf1))
	t.Logf(hex.EncodeToString(hugebuf1))
}
