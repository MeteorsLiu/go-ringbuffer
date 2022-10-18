package ringbuffer

import (
	"crypto/rand"
	"crypto/sha1"
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

	hugebuf := make([]byte, 65536)
	hugebuf1 := make([]byte, 65536)
	n, _ := io.ReadFull(rand.Reader, hugebuf)
	hugebuf = hugebuf[:n]
	hash := sha1.Sum(hugebuf)
	t.Logf("First, %s", hex.EncodeToString(hash[:]))
	t.Log(r.Write(hugebuf))
	t.Log(r.Read(hugebuf1))
	hash = sha1.Sum(hugebuf1)
	t.Logf("Third, %s", hex.EncodeToString(hash[:]))
}
