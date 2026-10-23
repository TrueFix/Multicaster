package rtmp

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"math/bits"
	"net"
	"net/url"
	"strings"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	msg "github.com/bluenviron/gortmplib/pkg/message"
)

// publishClient uses gortmplib's wire codecs, but negotiates the stream ID
// instead of the high-level client's stream-1-only publishing implementation.
type publishClient struct {
	conn     net.Conn
	wire     *msg.ReadWriter
	streamID uint32 // Numeric ID from createStream, not the codec's wire-order ID.
}

func (c *publishClient) NetConn() net.Conn          { return c.conn }
func (c *publishClient) Close()                     { _ = c.conn.Close() }
func (c *publishClient) Read() (msg.Message, error) { return c.wire.Read() }
func (c *publishClient) Write(m msg.Message) error  { return c.wire.Write(m) }
func (c *publishClient) messageStreamID() uint32    { return bits.ReverseBytes32(c.streamID) }

func (c *publishClient) initialize(ctx context.Context, u *url.URL, dial func(context.Context, string, string) (net.Conn, error)) error {
	port := u.Port()
	if port == "" {
		port = "1935"
		if u.Scheme == "rtmps" {
			port = "443"
		}
	}
	conn, err := dial(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return err
	}
	c.conn = conn
	// Closing raw TCP interrupts TLS handshakes and blocked protocol writes too.
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { conn.Close(); close(finished) })
	defer func() {
		if !stop() {
			<-finished
		}
	}()
	if u.Scheme == "rtmps" {
		c.conn = tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	}
	bc := bytecounter.NewReadWriter(c.conn)
	if _, _, err := handshake.DoClient(bc, false, false); err != nil {
		return err
	}
	c.wire = msg.NewReadWriter(bc, bc, false)
	for _, m := range []msg.Message{
		&msg.SetWindowAckSize{Value: 2500000},
		&msg.SetPeerBandwidth{Value: 2500000, Type: 2},
		&msg.SetChunkSize{Value: 65536},
	} {
		if err := c.Write(m); err != nil {
			return err
		}
	}
	target := *u
	target.Fragment, target.RawFragment = "", ""
	if err := c.command("connect", 1, 0, amf0.Object{
		{Key: "app", Value: strings.TrimPrefix(target.RequestURI(), "/")},
		{Key: "tcUrl", Value: target.String()},
		{Key: "flashVer", Value: "FMLE/3.0 (compatible; Multicaster)"},
		{Key: "objectEncoding", Value: float64(0)},
	}); err != nil {
		return err
	}
	if _, err := c.result(1); err != nil {
		return err
	}
	if err := c.command("releaseStream", 2, 0, nil, u.Fragment); err != nil {
		return err
	}
	if err := c.command("FCPublish", 3, 0, nil, u.Fragment); err != nil {
		return err
	}
	if err := c.command("createStream", 4, 0, nil); err != nil {
		return err
	}
	res, err := c.result(4)
	if err != nil {
		return err
	}
	if len(res.Arguments) < 2 {
		return errors.New("missing publishing stream ID")
	}
	id, ok := res.Arguments[1].(float64)
	if !ok || math.IsNaN(id) || id < 1 || id > math.MaxUint32 || id != math.Trunc(id) {
		return errors.New("invalid publishing stream ID")
	}
	c.streamID = uint32(id)
	if err := c.command("publish", 5, c.messageStreamID(), nil, u.Fragment, "live"); err != nil {
		return err
	}
	for {
		m, err := c.Read()
		if err != nil {
			return err
		}
		if err := providerError(m, c.messageStreamID()); err != nil {
			return err
		}
		cmd, ok := m.(*msg.CommandAMF0)
		if ok && cmd.Name == "onStatus" && (cmd.MessageStreamID == 0 || cmd.MessageStreamID == c.messageStreamID()) && commandString(cmd, "code") == "NetStream.Publish.Start" {
			return nil
		}
	}
}

func (c *publishClient) command(name string, transaction int, streamID uint32, args ...any) error {
	return c.Write(&msg.CommandAMF0{ChunkStreamID: 3, MessageStreamID: streamID, Name: name, CommandID: transaction, Arguments: args})
}

func (c *publishClient) result(transaction int) (*msg.CommandAMF0, error) {
	for {
		m, err := c.Read()
		if err != nil {
			return nil, err
		}
		if err := providerError(m, 0); err != nil {
			return nil, err
		}
		if cmd, ok := m.(*msg.CommandAMF0); ok && cmd.Name == "_result" && cmd.CommandID == transaction {
			return cmd, nil
		}
	}
}

func commandString(cmd *msg.CommandAMF0, key string) string {
	if len(cmd.Arguments) < 2 {
		return ""
	}
	var object amf0.Object
	switch v := cmd.Arguments[1].(type) {
	case amf0.Object:
		object = v
	case amf0.ECMAArray:
		object = amf0.Object(v)
	}
	value, _ := object.GetString(key)
	return value
}

// Only fixed messages leave this function: server descriptions and even unknown
// codes can contain stream keys or signed URLs.
func providerError(m msg.Message, streamID uint32) error {
	cmd, ok := m.(*msg.CommandAMF0)
	if !ok || (cmd.MessageStreamID != 0 && cmd.MessageStreamID != streamID) {
		return nil
	}
	if cmd.Name == "_error" {
		// FMLE's releaseStream/FCPublish are optional extensions. Some RTMP
		// servers reject those transactions but still support normal publish.
		if cmd.CommandID == 2 || cmd.CommandID == 3 {
			return nil
		}
		return errors.New("provider rejected RTMP command")
	}
	if cmd.Name != "onStatus" {
		return nil
	}
	if level := commandString(cmd, "level"); level == "error" {
		return errors.New("provider reported a publishing error")
	}
	switch commandString(cmd, "code") {
	case "NetStream.Publish.BadName", "NetStream.Publish.Failed", "NetStream.Publish.Denied", "NetStream.Failed", "NetConnection.Connect.Rejected", "NetConnection.Connect.Failed", "NetConnection.Connect.Closed", "NetStream.Unpublish.Success", "NetStream.Publish.Stop":
		return errors.New("provider ended or rejected publishing")
	}
	return nil
}
