package rtmp

import (
	"net"
	"sync"
	"time"

	msg "github.com/bluenviron/gortmplib/pkg/message"
)

const gracefulStopTimeout = time.Second

// A cancellation starts a bounded drain, not an immediate socket close. The
// underlying TCP watchdog also interrupts a blocked media/TLS write, regardless
// of a concurrent callback changing the client's write deadline.
type publishShutdown struct {
	client    *publishClient
	transport net.Conn
	once      sync.Once
	deadline  time.Time
	watchdog  *time.Timer
}

func (s *publishShutdown) begin() {
	s.once.Do(func() {
		s.deadline = time.Now().Add(gracefulStopTimeout)
		s.watchdog = time.AfterFunc(gracefulStopTimeout, func() { s.transport.Close() })
		s.client.NetConn().SetWriteDeadline(s.deadline)
	})
}
func (s *publishShutdown) close() {
	s.begin()
	defer s.watchdog.Stop()
	defer s.transport.Close()
	s.client.Close()
}

// These are publisher-to-server commands. UserControlStreamEOF/onStatus are
// server notifications and must not be impersonated by the publisher.
func unpublish(client *publishClient, key string, lastVideoTimestamp uint32, hasVideo bool) error {
	if hasVideo {
		if err := client.Write(&msg.Video{ChunkStreamID: 6, MessageStreamID: client.messageStreamID(), DTS: time.Duration(lastVideoTimestamp) * time.Millisecond, FrameType: msg.VideoFrameTypeKeyFrame, Codec: msg.CodecH264, PacketType: msg.VideoPacketTypeEOS}); err != nil {
			return err
		}
	}
	if err := client.Write(&msg.CommandAMF0{ChunkStreamID: 3, Name: "FCUnpublish", CommandID: 0, Arguments: []any{nil, key}}); err != nil {
		return err
	}
	return client.Write(&msg.CommandAMF0{ChunkStreamID: 3, Name: "deleteStream", CommandID: 0, Arguments: []any{nil, float64(client.streamID)}})
}
