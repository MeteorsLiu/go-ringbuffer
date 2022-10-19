package ringbuffer

import (
	"errors"
	"io"
	"sync/atomic"
	"time"
)

const (
	DEFAULT_RING_LEN = 1024
	SMALL_BUF_SIZE   = 1024
	MIDDLE_BUF_SIZE  = 2048
	// By default, the PAGE_SIZE is 4096 Bytes.
	// so may be more efficient for slices?
	BIG_BUF_SIZE = 4096
	// 2MB per HUGE PAGE
	HUGE_PAGE_SIZE = 1024 * 1024 * 2
	// 5 Minute per cleaning period
	SWEEP_POOL_EXPIRE = 300
)

var (
	POOL_EMPTY   = errors.New("pool is empty")
	BUFFER_EMPTY = errors.New("buffer is empty")
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
	cleaning int64
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
	defer func() {
		atomic.CompareAndSwapInt64(&p.cleaning, atomic.LoadInt64(&p.cleaning), time.Now().Unix())
		atomic.AddInt32(&p.flushing, -1)
	}()
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
				return
			}
		}
	case WRITING_POOL:
		for len(p.w) == 0 {
			select {
			case _ = <-p.w:
			default:
				return
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
				return
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
				return
			}
		}
	}
}
func (b *buffer) read(pool *rwPool, buf []byte) (n int) {
	n = copy(buf, b.buf)
	b.pos += n
	if b.pos == len(b.buf) {
		select {
		case pool.w <- b:
		default:
		}
	} else {
		// if reading is not done, put it into the leftover pool
		// let's read it again
		b.buf = b.buf[b.pos:]
		select {
		case pool.rleftover <- b:
		default:
		}
	}
	return
}

func (b *buffer) write(pool *rwPool, buf []byte) (n int) {
	n = copy(b.buf[0:cap(buf)], buf)
	if n == 0 {
		panic("error copy")
	}
	b.pos = 0
	b.buf = b.buf[:n]
	select {
	case pool.r <- b:
	default:
	}
	return
}

func (b *buffer) write_leftover(pool *rwPool, buf []byte) (n int) {
	n = copy(b.buf[0:cap(buf)], buf)
	if n == 0 {
		panic("error copy")
	}
	b.pos = 0
	b.buf = b.buf[:n]
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

	select {
	case buf = <-r.pool.wleftover:
	default:
		buf = r.grabReadBuffer()
	}

	return buf
}

func (r *Ring) doSweep() {
	if atomic.LoadInt32(&r.pool.wrefcnt) == 0 &&
		len(r.pool.wleftover) > 0 {
		r.pool.Flush(WRITING_LEFT)
	}
}

func (r *Ring) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		err = BUFFER_EMPTY
		return
	}
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
		if buf == nil || len(buf.buf) == 0 {
			return
		}
		nr := buf.read(&r.pool, b[n:])
		n += nr
	}
	if r.pool.cleaning == 0 {
		r.pool.cleaning = time.Now().Unix()
	}
	// flush pool if wrefcnt reaches zero
	// flush process shouldn't be blocked otherwise it will pause the reading.
	if time.Now().Unix()-r.pool.cleaning >= SWEEP_POOL_EXPIRE {
		go r.doSweep()
	}
	return
}

func allocs(size int) int {
	if size <= SMALL_BUF_SIZE {
		return SMALL_BUF_SIZE
	} else if size <= MIDDLE_BUF_SIZE {
		return MIDDLE_BUF_SIZE
	} else if size <= BIG_BUF_SIZE {
		return BIG_BUF_SIZE
	} else {
		return HUGE_PAGE_SIZE
	}
}

func (r *Ring) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		err = BUFFER_EMPTY
		return
	}
	var buf *buffer
	select {
	case buf = <-r.pool.w:
	default:
		buf = &buffer{
			buf: make([]byte, allocs(len(b))),
		}

	}
	n = buf.write(&r.pool, b)
	if n < len(b) {
		// notice another reader not to flush the pool.
		atomic.AddInt32(&r.pool.wrefcnt, 1)
		defer atomic.AddInt32(&r.pool.wrefcnt, -1)
	}
	for n < len(b) {
		select {
		case buf = <-r.pool.w:
		default:
			buf = &buffer{
				buf: make([]byte, allocs(len(b))),
			}

		}
		nw := buf.write_leftover(&r.pool, b[n:])
		n += nw
	}
	return
}
