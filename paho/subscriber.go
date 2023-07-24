package paho

import (
	"context"
	"fmt"

	"github.com/eclipse/paho.golang/packets"
)

// InternalSend is the structure that we use to keep track of subscribe and unsubscribe
// operations that are in progress.
// TODO: Since this is internal, should it use a different naming convention (internalSend or _internalSend)?
type InternalSend struct {
	packet *packets.ControlPacket
	ackReceived chan *packets.ControlPacket
}

// SubUnsubHandler is the object that makes MQTT Subscribe and Unsubscribe operations appear
// as though they are synchroneous
type SubUnsubHandler struct {
	// All of these things (except `debug`) are created by us in `NewSubUnsubHandler` and cleaned
	// up in `Cleanup`. Therefore, we can assume they will always exist and will never change, 
	// and so they don't need any thread sync protection.

	// Receive is the channel that outsiders use to push ControlPackets into our handler.
	Receive chan *packets.ControlPacket

	// `internalSend` and `internalCancel` are channels that are used to communicate between user 
	// code (such as `Subscribe`) to this handler's `Run` loop.
	internalSend chan *InternalSend
	internalCancel chan uint16

	// `internalStop` is a channel that is used to communicate from the `Run` loop to user code
	// such as `Subscribe`.
	internalStop chan bool

	// `debug` is an abomination. We should not rely on the `debug` object from `Client`, but
	// we do, and that object can change, so we have to be ready for it to change.
	debug		Logger
}

// Create a new SubUnsubHandler object configured with default values
func NewSubUnsubHandler() *SubUnsubHandler {
	return &SubUnsubHandler{
		Receive: make(chan *packets.ControlPacket),
		internalSend: make(chan *InternalSend),
		internalCancel: make(chan uint16),
		internalStop: make(chan bool),
		debug: NOOPLogger{},
	}
}

// Clean up the SubUnsubHandler object.  
func (s *SubUnsubHandler) Cleanup() {
	// This function makes heavy use of `defer` so any failures will not stop later cleanup code 
	// from executing.  (ie. if `close(s.internalStop)` panics, `close(s.Receive)` should still
	// be called.
	defer close(s.internalStop)
	defer close(s.Receive)
	defer close(s.internalSend)
	defer close(s.internalCancel)
	defer func() {
		s.internalStop = nil
		s.Receive = nil
		s.internalSend = nil
		s.internalCancel = nil
	}()
	defer s.debug.Println("Done cleaning up SubUnsubHandler")
}

// `Run` is the main loop for this handler handler. It is meant to be called from a goroutine.
func (s *SubUnsubHandler) Run(send chan<- *packets.ControlPacket, stop <-chan struct{}, error chan<- error) {
	// Note 1: This object has strange lifetime semantics.  Once `Run` returns, the object gets
	// cleaned up (via `defer s.Cleanup()`, so it is effectively "dead".

	// Note 2: We already created the channels that the caller uses to pass packets in (in 
	// `NewSubUnsubChannel` above. Any channels that we use to pass events or messages out get passed
	// in as parameters to `Run` and never stored. This way we can ensure that nothing changes 
	// underneath us.

	// Note 3: Any objects that might normally need thread protection (like the `ackMap` object) are 
	// stored as stack parameters in this function and never passed around. Because nobody else is
	// using them, we don't need to worry about protecting them. 
	// lock them.

	// Keep it simple for now. This could use a PacketID allocater later. 
	ackMap := make(map[uint16](*InternalSend))
	var nextPacketID uint16 = 1

	defer s.Cleanup()

	s.debug.Println("SubUnsubHandler running")

	for {
		select {
		case <- stop:
			s.debug.Println("SubUnsubHandler stopped")
			// We don't have to worry about cleaning up ackMap.  Any subscribe or unsubscribe 
			// operations have been handled by the Subscribe and Unsubscribe calls.

			// Since were returning, the `defer s.Cleanup()` code above ensures that Subscribe
			// and Unsubscribe operations get stopped (via `close s.internalStop`)
			return
		case is := <- s.internalSend:
			// This is a message from either `Subscribe` or `Unsubscribe`. This code needs to be
			// here because 1. we are the only function allowed to push to `send` and 2. we are the
			// only function alloed to modify `ackMap`.
			packetID := nextPacketID
			// TODO what happens on overflow. Does it wrap around again?
			nextPacketID++
			ackMap[packetID] = is

			is.packet.SetPacketID(packetID)

			// Wrap this in a goroutine since we don't care when this completes and we'll catch
			// failures some other way (probably via timeout or failure reading from the socket).
			go func() {
				s.debug.Printf("SubUnsubHandler sending %v packetId %v", is.packet.PacketType(), packetID)
				send <- is.packet
			}()
		case pid := <- s.internalCancel:
			// This is anoher message from `Subscribe` or `Unsubscribe` that tells when a sub or unsub
			// operation has been cancelled.
			delete(ackMap, pid)
		case a := <- s.Receive:
			// This is a message from the socket.  It should be either a `packets.Suback` or 
			// `packets.Unsuback` object since they are the only ones we need to worry about.
			s.debug.Printf("SubUnsubHandler received %v packetId %v", a.PacketType(), a.PacketID())
			is, ok := ackMap[a.PacketID()]
			if ok {
				delete(ackMap, a.PacketID())
				go func() {
					// Trigger the completion of the `Subscribe` or `Unsubscribe` call.  Do this in a
					// goroutine because we don't need to block this loop waiting for any receivers to 
					// become ready.
					is.ackReceived <- a
				}()
			}
		}
	}
}

// Send a subscribe packet and wait for the matching suback to return. This is safe to call from
// a goroutine.
func (s *SubUnsubHandler) Subscribe(ctx context.Context, subscribe *packets.Subscribe) (*packets.Suback, error) {
	ackReceived := make(chan *packets.ControlPacket)
	defer close(ackReceived)

	packet := subscribe.ToControlPacket()
	
	// Push the packet into the Run loop.  Do this in a goroutine so we don't need to block waiting 
	// for the run loop to be ready to receive.
	go func() {
		s.internalSend <- &InternalSend{packet: packet, ackReceived: ackReceived}
	}()

	// One of three things could happen now. Handle them.
	select {
	case <- s.internalStop:
		s.debug.Println("Failing Subscribe call because subscriber stopped")
		return nil, fmt.Errorf("Client stopped before suback received")
	case <- ctx.Done():
		return nil, fmt.Errorf("Operation Cancelled")
	case a := <- ackReceived:
		s.debug.Println("Suback received")
		return a.Content.(*packets.Suback), nil
	}
}

// Send an unsubscribe packet and wait for the matching unsuback to return. This is safe to call
// from a goroutine.
func (s *SubUnsubHandler) Unsubscribe(ctx context.Context, unsubscribe *packets.Unsubscribe) (*packets.Unsuback, error) {

	ackReceived := make(chan *packets.ControlPacket)
	defer close(ackReceived)

	packet := unsubscribe.ToControlPacket()

	// Push the packet into the Run loop.  Do this in a goroutine so we don't need to block waiting 
	// for the run loop to be ready to receive.
	go func() {
		s.internalSend <- &InternalSend{packet: packet, ackReceived: ackReceived}
	}()

	// One of three things could happen now. Handle them.
	select {
	case <- s.internalStop:
		s.debug.Println("Failing Unsubscribe call because subscriber stopped")
		return nil, fmt.Errorf("Client stopped before unsuback received")
	case <- ctx.Done():
		return nil, fmt.Errorf("Operation Cancelled")
	case a := <- ackReceived:
		// TODO: deal with invalid type here
		s.debug.Println("Unsuback received")
		return a.Content.(*packets.Unsuback), nil
	}
}

// SetDebugLogger takes an instance of the paho Logger interface
// and sets it to be used by the debug log endpoint
func (c *SubUnsubHandler) SetDebugLogger(l Logger) {
	c.debug = l
}
