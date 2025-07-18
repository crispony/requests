package requests

import (
	"context"
	"errors"
	"io"
	"iter"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gospider007/tools"
)

var maxRetryCount = 10

type Conn interface {
	CloseWithError(err error) error
	DoRequest(*http.Request, []interface {
		Key() string
		Val() any
	}) (*http.Response, context.Context, error)
	Stream() io.ReadWriteCloser
}
type connecotr struct {
	forceCtx context.Context //force close
	forceCnl context.CancelCauseFunc
	Conn     Conn
	c        net.Conn
	proxys   []Address
}

func (obj *connecotr) withCancel(forceCtx context.Context) {
	obj.forceCtx, obj.forceCnl = context.WithCancelCause(forceCtx)
}
func (obj *connecotr) Close() error {
	return obj.CloseWithError(errors.New("connecotr Close close"))
}
func (obj *connecotr) CloseWithError(err error) error {
	err = obj.Conn.CloseWithError(err)
	if obj.c != nil {
		return obj.c.Close()
	}
	return err
}

func (obj *connecotr) wrapBody(task *reqTask) {
	body := new(wrapBody)
	if task.reqCtx.response.Body == nil {
		task.reqCtx.response.Body = http.NoBody
	}
	rawBody := task.reqCtx.response.Body
	body.rawBody = rawBody
	body.conn = obj
	task.reqCtx.response.Body = body
	task.reqCtx.response.Request = task.reqCtx.request
}

func (obj *connecotr) httpReq(task *reqTask, done chan struct{}) (err error) {
	defer close(done)
	task.reqCtx.response, task.bodyCtx, err = obj.Conn.DoRequest(task.reqCtx.request, task.reqCtx.option.orderHeaders.Data())
	if task.reqCtx.response != nil {
		obj.wrapBody(task)
	}
	if err != nil {
		err = tools.WrapError(err, "roundTrip error")
	}
	return
}

func (obj *connPool) taskMain(conn *connecotr, task *reqTask) (err error) {
	defer func() {
		if err != nil && task.reqCtx.option.ErrCallBack != nil {
			task.reqCtx.err = err
			if err2 := task.reqCtx.option.ErrCallBack(task.reqCtx); err2 != nil {
				task.isNotice = false
				task.disRetry = true
				err = err2
			}
		}
		if err == nil {
			task.cnl(tools.ErrNoErr)
		} else {
			task.cnl(err)
		}
		if err == nil && task.reqCtx.response != nil && task.reqCtx.response.Body != nil && task.bodyCtx != nil {
			select {
			case <-conn.forceCtx.Done():
				err = context.Cause(conn.forceCtx)
			case <-task.reqCtx.Context().Done():
				if context.Cause(task.reqCtx.Context()) != tools.ErrNoErr {
					err = context.Cause(task.reqCtx.Context())
				}
				if err == nil && task.reqCtx.response.StatusCode == 101 {
					select {
					case <-conn.forceCtx.Done():
						err = context.Cause(conn.forceCtx)
					case <-task.bodyCtx.Done():
						if context.Cause(task.bodyCtx) != tools.ErrNoErr {
							err = context.Cause(task.bodyCtx)
						}
					}
				}
			case <-task.bodyCtx.Done():
				if context.Cause(task.bodyCtx) != tools.ErrNoErr {
					err = context.Cause(task.bodyCtx)
				}
			}
		}
		if err != nil {
			conn.CloseWithError(tools.WrapError(err, "taskMain close with error"))
		}
	}()
	select {
	case <-conn.forceCtx.Done(): //force conn close
		err = context.Cause(conn.forceCtx)
		task.enableRetry = true
		task.isNotice = true
		return
	default:
	}
	done := make(chan struct{})
	go conn.httpReq(task, done)
	select {
	case <-conn.forceCtx.Done(): //force conn close
		err = tools.WrapError(context.Cause(conn.forceCtx), "taskMain delete ctx error: ")
	case <-time.After(task.reqCtx.option.ResponseHeaderTimeout):
		err = errors.New("ResponseHeaderTimeout error: ")
	case <-task.ctx.Done():
		err = context.Cause(task.ctx)
	case <-done:
		if task.reqCtx.response == nil {
			err = context.Cause(task.ctx)
			if err == nil {
				err = errors.New("body done response is nil")
			}
		}
		if task.reqCtx.option.Logger != nil {
			task.reqCtx.option.Logger(Log{
				Id:   task.reqCtx.requestId,
				Time: time.Now(),
				Type: LogType_ResponseHeader,
				Msg:  "response header",
			})
		}
	}
	return
}

type connPool struct {
	forceCtx  context.Context
	forceCnl  context.CancelCauseFunc
	tasks     chan *reqTask
	connPools *connPools
	connKey   string
	total     atomic.Int64
}
type connPools struct {
	connPools sync.Map
}

func newConnPools() *connPools {
	return new(connPools)
}
func (obj *connPools) get(task *reqTask) *connPool {
	val, ok := obj.connPools.Load(task.key)
	if !ok {
		return nil
	}
	return val.(*connPool)
}
func (obj *connPools) set(task *reqTask, pool *connPool) {
	obj.connPools.Store(task.key, pool)
}
func (obj *connPools) del(key string) {
	obj.connPools.Delete(key)
}
func (obj *connPools) Range() iter.Seq2[string, *connPool] {
	return func(yield func(string, *connPool) bool) {
		obj.connPools.Range(func(key, value any) bool {
			return yield(key.(string), value.(*connPool))
		})
	}
}

func (obj *connPool) rwMain(done chan struct{}, conn *connecotr) {
	conn.withCancel(obj.forceCtx)
	defer func() {
		conn.CloseWithError(errors.New("connPool rwMain close"))
		obj.total.Add(-1)
		if obj.total.Load() <= 0 {
			obj.close(errors.New("conn pool close"))
		}
	}()
	close(done)
	for {
		select {
		case <-conn.forceCtx.Done(): //force close conn
			return
		case task := <-obj.tasks: //recv task
			if task == nil {
				return
			}
			err := obj.taskMain(conn, task)
			if err != nil {
				return
			}
		}
	}
}
func (obj *connPool) close(err error) {
	obj.connPools.del(obj.connKey)
	obj.forceCnl(tools.WrapError(err, "connPool close"))
}

func newSSHConn(sshCon net.Conn, rawCon net.Conn) *sshConn {
	return &sshConn{sshCon: sshCon, rawCon: rawCon}
}

type sshConn struct {
	sshCon net.Conn
	rawCon net.Conn
}

func (obj *sshConn) Read(b []byte) (n int, err error) {
	return obj.sshCon.Read(b)
}

func (obj *sshConn) Write(b []byte) (n int, err error) {
	return obj.sshCon.Write(b)
}

func (obj *sshConn) Close() error {
	return obj.sshCon.Close()
}
func (obj *sshConn) LocalAddr() net.Addr {
	return obj.sshCon.LocalAddr()
}
func (obj *sshConn) RemoteAddr() net.Addr {
	return obj.sshCon.RemoteAddr()
}
func (obj *sshConn) SetDeadline(deadline time.Time) error {
	return obj.rawCon.SetDeadline(deadline)
}
func (obj *sshConn) SetReadDeadline(deadline time.Time) error {
	return obj.rawCon.SetReadDeadline(deadline)
}

func (obj *sshConn) SetWriteDeadline(deadline time.Time) error {
	return obj.rawCon.SetWriteDeadline(deadline)
}
