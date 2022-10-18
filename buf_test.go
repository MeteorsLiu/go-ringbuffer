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
	t.Log(smallbuf)

	hugebuf := make([]byte, 65536)
	io.ReadFull(rand.Reader, hugebuf)
	hash := sha1.Sum(hugebuf)
	t.Logf("First, %s", hex.EncodeToString(hash[:]))
	t.Log(r.Write(hugebuf))
	io.ReadFull(rand.Reader, hugebuf)
	hash = sha1.Sum(hugebuf)
	t.Logf("Second, %s", hex.EncodeToString(hash[:]))
	t.Log(r.Read(hugebuf))
	hash = sha1.Sum(hugebuf)
	t.Logf("Third, %s", hex.EncodeToString(hash[:]))
}
