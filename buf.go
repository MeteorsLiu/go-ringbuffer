package ringbuffer

import (
	"errors"
	"io"
	"sync/atomic"
)

const (
	DEFAULT_RING_LEN = 1024

	// By default, the PAGE_SIZE is 4096.
	// so may be more efficient for slices?
	DEFAULT_BUF_SIZE = 4096
)

var (
	POOL_EMPTY = errors.New("pool is empty")
)

type buffer struct {
	buf []byte
	pos int
}

const (
	READING_POOL = iota
	WRITING_POOL
	READING_LEFT
	WRITING_LEFT
)

type rwPool struct {
	r         chan *buffer
	w         chan *buffer
	rleftover chan *buffer
	wleftover chan *buffer
	// reference counter
	// if the wrefcnt reaches zero, the wleftover pool will be swept forcely.
	// it avoids the memory leak.
	wrefcnt  int32
	flushing int32
}
type Ring struct {
	pool         rwPool
	len          int
	blockingMode bool
}

func New(isBlock bool, ringOpts ...int) io.ReadWriter {
	size := DEFAULT_RING_LEN
	if len(ringOpts) > 0 {
		size = ringOpts[0]
	}
	r := &Ring{
		blockingMode: isBlock,
		len:          size,
	}
	r.pool.r = make(chan *buffer, size)
	r.pool.w = make(chan *buffer, size)
	r.pool.rleftover = make(chan *buffer, size)
	r.pool.wleftover = make(chan *buffer, size)
	return r
}

func (p *rwPool) Flush(pool int) {
	if atomic.LoadInt32(&p.flushing) > 0 {
		return
	}
	atomic.AddInt32(&p.flushing, 1)
	defer atomic.AddInt32(&p.flushing, -1)
	switch pool {
	case READING_POOL:
		for len(p.r) == 0 {
			select {
			case b := <-p.r:
				b.pos = 0
				b.buf = b.buf[0:]
				select {
				case p.w <- b:
				default:
				}
			default:
				break
			}
		}
	case WRITING_POOL:
		for len(p.w) == 0 {
			select {
			case _ = <-p.w:
			default:
				break
			}
		}
	case READING_LEFT:
		for len(p.rleftover) == 0 {
			select {
			case b := <-p.rleftover:
				b.pos = 0
				b.buf = b.buf[0:]
				select {
				case p.w <- b:
				default:
				}
			default:
				break
			}
		}
	case WRITING_LEFT:
		for len(p.wleftover) == 0 {
			select {
			case b := <-p.wleftover:
				b.pos = 0
				b.buf = b.buf[0:]
				select {
				case p.w <- b:
				default:
				}
			default:
				break
			}
		}
	}
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
		// if reading is not done, put it into the leftover pool
		// let's read it again
		select {
		case pool.rleftover <- b:
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

func (b *buffer) write_leftover(pool *rwPool, buf []byte) (n int) {
	if b.pos == len(b.buf) {
		b.pos = 0
	}
	if atomic.LoadInt32(&pool.wrefcnt) == 0 && len(pool.wleftover) > 0 {
		pool.Flush(WRITING_LEFT)
	}
	n = copy(b.buf[b.pos:], buf)
	b.pos += n
	select {
	case pool.wleftover <- b:
	default:
	}
	return
}

func (r *Ring) grabReadBuffer() *buffer {
	var buf *buffer

	// Reading Order:
	// Read pool leftover buffer -> First
	// Read pool buffer -> Second
	// Write Pool Leftover(Slice Buffer) -> Third
	if r.blockingMode {
		// we must pull it from leftover pool first, which makes sure the reading order is correct.
		select {
		case buf = <-r.pool.rleftover:
		default:
			// if there is nothing in leftover pool, try to grab it in the reading pool
			buf = <-r.pool.r
		}
	} else {
		// non-blocking
		// if buffer pool is empty, return the error immediately.
		select {
		case buf = <-r.pool.rleftover:
		default:
			// if there is nothing in leftover pool, try to grab it in the reading pool
			select {
			case buf = <-r.pool.r:
			default:
				return nil

			}
		}
	}
	return buf
}

func (r *Ring) grabLeftoverBuffer() *buffer {
	var buf *buffer
	if atomic.LoadInt32(&r.pool.flushing) > 0 {
		return nil
	}
	atomic.AddInt32(&r.pool.wrefcnt, 1)
	defer atomic.AddInt32(&r.pool.wrefcnt, -1)
	if r.blockingMode {
		select {
		case buf = <-r.pool.wleftover:
		default:
			// if there is nothing in leftover pool, try to grab it in the reading pool
			buf = <-r.pool.r
		}
	} else {
		// non-blocking
		// if buffer pool is empty, return the error immediately.
		select {
		case buf = <-r.pool.wleftover:
		default:
			// if there is nothing in leftover pool, try to grab it in the reading pool
			select {
			case buf = <-r.pool.r:
			default:
				return nil

			}
		}
	}
	return buf
}

func (r *Ring) Read(b []byte) (n int, err error) {
	var buf *buffer
	buf = r.grabReadBuffer()
	if buf == nil {
		err = POOL_EMPTY
		return
	}
	n = buf.read(&r.pool, b)
	// if reading is not done, there will no more reading leftover buffer produced.
	// so try to grab the writing leftover buffer
	for n < len(b) {
		buf = r.grabLeftoverBuffer()
		if buf == nil {
			return
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
	// no to clean the pool while writing the leftover
	atomic.AddInt32(&r.pool.wrefcnt, 1)
	defer atomic.AddInt32(&r.pool.wrefcnt, -1)
	for n < len(b) {
		select {
		case buf = <-r.pool.w:
		default:
			buf = &buffer{
				buf: make([]byte, DEFAULT_BUF_SIZE),
			}
		}
		nw := buf.write_leftover(&r.pool, b)
		n += nw
	}
	return
}
