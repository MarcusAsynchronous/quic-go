package flowcontrol

import "github.com/lucas-clemente/quic-go/internal/protocol"

type flowController interface {
	// for sending
	SendWindowSize() protocol.ByteCount
	IsBlocked() bool
	UpdateSendWindow(protocol.ByteCount)
	AddBytesSent(protocol.ByteCount)
	// for receiving
	AddBytesRead(protocol.ByteCount)
	GetWindowUpdate() protocol.ByteCount // returns 0 if no update is necessary
}

type StreamFlowController interface {
	flowController
	// for receiving
	UpdateHighestReceived(offset protocol.ByteCount, final bool) error
}

type ConnectionFlowController interface {
	flowController
}

type connectionFlowControllerI interface {
	ConnectionFlowController
	// The following two methods are not supposed to be called from outside this packet, but are needed internally
	// for sending
	EnsureMinimumWindowIncrement(protocol.ByteCount)
	// for receiving
	IncrementHighestReceived(protocol.ByteCount) error
}
