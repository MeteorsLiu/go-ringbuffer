package ringbuffer

import (
	"errors"
)

const (
	DEFAULT_RING_LEN = 1024
	DEFAULT_BUF_SIZE = 4096
)

var (
	POOL_EMPTY = errors.New("pool is empty")
)

type buffer struct {
	buf []byte
	pos int
}

type Ring struct {
	rpool        chan *buffer
	wpool        chan *buffer
	len          int
	blockingMode bool
}

func New(isBlock bool, ringOpts ...int) *Ring {
	size := DEFAULT_RING_LEN
	if len(ringOpts) > 0 {
		size = ringOpts[0]
	}
	r := &Ring{
		blockingMode: isBlock,
		len:          size,
		rpool:        make(chan *buffer, size),
		wpool:        make(chan *buffer, size),
	}
	return r
}

func (b *buffer) read(wpool chan *buffer, buf []byte) (n int) {
	if b.pos == 0 {
		return
	}
	n = copy(buf, b.buf[b.pos:])
	b.pos -= n
	select {
	case wpool <- b:
	default:
	}
	return
}

func (b *buffer) write(rpool chan *buffer, buf []byte) (n int) {
	if b.pos == len(b.buf) {
		b.pos = 0
	}
	n = copy(b.buf[b.pos:], buf)
	b.pos += n
	select {
	case rpool <- b:
	default:
	}
	return
}

func (r *Ring) Read(b []byte) (n int, err error) {
	var buf *buffer
	if r.blockingMode {
		buf = <-r.rpool
	} else {
		// non-blocking
		// if buffer pool is empty, return the error immediately.
		select {
		case buf = <-r.rpool:
		default:
			err = POOL_EMPTY
			return
		}
	}
	n = buf.read(r.wpool, b)

	for n < len(b) {
		if r.blockingMode {
			buf = <-r.rpool
		} else {
			// non-blocking
			// if buffer pool is empty, return immediately.
			select {
			case buf = <-r.rpool:
			default:
				return
			}
		}
		nr := buf.read(r.wpool, b)
		n += nr
	}
	return
}

func (r *Ring) Write(b []byte) (n int, err error) {
	var buf *buffer
	select {
	case buf = <-r.wpool:
	default:
		buf = &buffer{
			buf: make([]byte, DEFAULT_BUF_SIZE),
		}
	}
	n = buf.write(r.rpool, b)
	for n < len(b) {
		select {
		case buf = <-r.wpool:
		default:
			buf = &buffer{
				buf: make([]byte, DEFAULT_BUF_SIZE),
			}
		}
		nw := buf.write(r.rpool, b)
		n += nw
	}
	return
}
