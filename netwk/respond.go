package netwk

import (
	"github.com/fmstephe/matching_engine/guid"
	"github.com/fmstephe/matching_engine/msg"
	"io"
	"net"
	"os"
	"time"
)

const RESEND_MILLIS = time.Duration(100) * time.Millisecond

type Responder struct {
	responses chan *msg.Message
	dispatch  chan *msg.Message
	unacked   map[int64]*msg.Message
	writer    io.WriteCloser
}

func NewResponder(writer io.WriteCloser) *Responder {
	return &Responder{unacked: make(map[int64]*msg.Message), writer: writer}
}

func (r *Responder) SetResponses(responses chan *msg.Message) {
	r.responses = responses
}

func (r *Responder) SetDispatch(dispatch chan *msg.Message) {
	r.dispatch = dispatch
}

func (r *Responder) Run() {
	defer r.shutdown()
	t := time.NewTimer(RESEND_MILLIS)
	for {
		select {
		case resp := <-r.responses:
			switch {
			case resp.Status != msg.NORMAL, resp.Route == msg.MATCHER_RESPONSE, resp.Route == msg.SERVER_ACK:
				r.writeResponse(resp)
			case resp.Route == msg.CLIENT_ACK:
				r.handleClientAck(resp)
			case resp.Route == msg.COMMAND && resp.Kind == msg.SHUTDOWN:
				return
			}
		case <-t.C:
			r.resend()
			t = time.NewTimer(RESEND_MILLIS)
		}
	}
}

func (r *Responder) handleClientAck(ca *msg.Message) {
	g := guid.MkGuid(ca.TraderId, ca.TradeId)
	m := r.unacked[g]
	if m != nil {
		delete(r.unacked, g)
	}
}

func (r *Responder) writeResponse(resp *msg.Message) {
	r.addToUnacked(resp)
	r.write(resp)
}

func (r *Responder) addToUnacked(resp *msg.Message) {
	if resp.Route == msg.MATCHER_RESPONSE {
		g := guid.MkGuid(resp.TraderId, resp.TradeId)
		r.unacked[g] = resp
	}
}

func (r *Responder) resend() {
	for _, resp := range r.unacked {
		r.write(resp)
	}
}

func (r *Responder) write(resp *msg.Message) {
	b := make([]byte, msg.SizeofMessage)
	resp.WriteTo(b)
	n, err := r.writer.Write(b)
	if err != nil {
		r.handleError(resp, err, msg.WRITE_ERROR)
	}
	if n != msg.SizeofMessage {
		r.handleError(resp, err, msg.SMALL_WRITE_ERROR)
	}
}

func (r *Responder) handleError(resp *msg.Message, err error, s msg.MsgStatus) {
	em := &msg.Message{}
	*em = *resp
	em.WriteStatus(s)
	r.dispatch <- em
	println(err.Error())
	if e, ok := err.(net.Error); ok && !e.Temporary() {
		os.Exit(1)
	}
}

func (r *Responder) shutdown() {
	r.writer.Close()
}
