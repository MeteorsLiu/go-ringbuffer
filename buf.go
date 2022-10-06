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
type rwPool struct {
	r chan *buffer
	w chan *buffer
}
type Ring struct {
	pool         rwPool
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
		pool: rwPool{
			r: make(chan *buffer, size),
			w: make(chan *buffer, size),
		},
	}
	return r
}

func (b *buffer) read(pool *rwPool, buf []byte) (n int) {
	if b.pos == 0 {
		return
	}
	n = copy(buf, b.buf[b.pos:])
	b.pos -= n
	if b.pos == 0 {
		select {
		case pool.w <- b:
		default:
		}
	} else {
		// if reading is not done, put it into the reading pool
		// let's read it again
		select {
		case pool.r <- b:
		default:
		}
	}
	return
}

func (b *buffer) write(pool *rwPool, buf []byte) (n int) {
	if b.pos == len(b.buf) {
		b.pos = 0
	}
	n = copy(b.buf[b.pos:], buf)
	b.pos += n
	select {
	case pool.r <- b:
	default:
	}
	return
}

func (r *Ring) Read(b []byte) (n int, err error) {
	var buf *buffer
	if r.blockingMode {
		buf = <-r.pool.r
	} else {
		// non-blocking
		// if buffer pool is empty, return the error immediately.
		select {
		case buf = <-r.pool.r:
		default:
			err = POOL_EMPTY
			return
		}
	}
	n = buf.read(&r.pool, b)

	for n < len(b) {
		if r.blockingMode {
			buf = <-r.pool.r
		} else {
			// non-blocking
			// if buffer pool is empty, return immediately.
			select {
			case buf = <-r.pool.r:
			default:
				return
			}
		}
		nr := buf.read(&r.pool, b)
		n += nr
	}
	return
}

func (r *Ring) Write(b []byte) (n int, err error) {
	var buf *buffer
	select {
	case buf = <-r.pool.w:
	default:
		buf = &buffer{
			buf: make([]byte, DEFAULT_BUF_SIZE),
		}
	}
	n = buf.write(&r.pool, b)
	for n < len(b) {
		select {
		case buf = <-r.pool.w:
		default:
			buf = &buffer{
				buf: make([]byte, DEFAULT_BUF_SIZE),
			}
		}
		nw := buf.write(&r.pool, b)
		n += nw
	}
	return
}
