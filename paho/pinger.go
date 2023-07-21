package paho

import (
	"fmt"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/packets"
)

// PingFailHandler is a type for the function that is invoked
// when the sending of a PINGREQ failed or when we have sent
// a PINGREQ to the server and not received a PINGRESP within
// the appropriate amount of time.
type PingFailHandler func(error)

// Pinger is an interface of the functions for a struct that is
// used to manage sending PingRequests and responding to
// PingResponses.
// Start() takes a net.Conn which is a connection over which an
// MQTT session has already been established, and a time.Duration
// of the keepalive setting passed to the server when the MQTT
// session was established.
// Stop() is used to stop the Pinger
// PingResp() is the function that is called by the Client when
// a PINGRESP is received
// SetDebug() is used to pass in a Logger to be used to log debug
// information, for example sharing a logger with the main client
type Pinger interface {
	Start(cPingresp chan *packets.Pingresp, cOutgoingPackets chan *packets.ControlPacket, pt time.Duration);
	Stop()
	SetDebug(Logger)
}

// PingHandler is the library provided default Pinger
type PingHandler struct {
	sync.Mutex
	stop             chan struct{}
	pingFailHandler  PingFailHandler
	debug            Logger
}

// DefaultPingerWithCustomFailHandler returns an instance of the
// default Pinger but with a custom PingFailHandler that is called
// when the client has not received a response to a PingRequest
// within the appropriate amount of time
func DefaultPingerWithCustomFailHandler(pfh PingFailHandler) *PingHandler {
	return &PingHandler{
		pingFailHandler: pfh,
		debug:           NOOPLogger{},
	}
}

// Start is the library provided Pinger's implementation of
// the required interface function()
func (p *PingHandler) Start(cPingresp chan *packets.Pingresp, cOutgoingPackets chan *packets.ControlPacket, pt time.Duration) {
	p.Lock()
	if p.stop != nil {
		select {
		case <-p.stop:
			// Stopped before, need to reset
		default:
			// Already running
			p.Unlock()
			return
		}
	}
	p.stop = make(chan struct{})
	p.Unlock()

	defer p.Stop()

	p.debug.Println("Starting pinger")
	sendNextPing := time.After(pt)
	pingExpired := make(<-chan time.Time)

	for {
		select {
		case <- p.stop:
			p.debug.Println("Pinger stopped")
			return
		case <- sendNextPing:
			p.debug.Println("Sending next ping")
			go func() {
				cOutgoingPackets <- packets.NewControlPacket(packets.PINGREQ)
			}()
			sendNextPing = nil
			pingExpired = time.After(pt / 2)
		case <- pingExpired:
			p.debug.Println("Pinger expired")
			p.pingFailHandler(fmt.Errorf("ping resp timed out"))
			return
		case <- cPingresp:
			p.debug.Println("Pingresp received.  Resetting.")
			sendNextPing = time.After(pt)
			pingExpired = nil
		}
	}
}

// Stop is the library provided Pinger's implementation of
// the required interface function()
func (p *PingHandler) Stop() {
	p.Lock()
	defer p.Unlock()
	if p.stop == nil {
		return
	}

	select {
	case <-p.stop:
		//Already stopped, do nothing
	default:
		p.debug.Println("pingHandler stopping")
		close(p.stop)
	}
}

// SetDebug is the library provided Pinger's implementation of
// the required interface function()
func (p *PingHandler) SetDebug(l Logger) {
	p.debug = l
}
