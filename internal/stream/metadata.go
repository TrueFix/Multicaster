package stream

import (
	"errors"

	"github.com/bluenviron/gortmplib/pkg/amf0"
)

// Metadata owns an immutable AMF0 encoding. Each consumer gets its own decoded
// object graph, so reconnecting outputs cannot mutate the cached metadata.
type Metadata struct{ encoded []byte }

func NewMetadata(payload amf0.Data) (*Metadata, error) {
	size, err := payload.MarshalSize()
	if err != nil {
		return nil, err
	}
	if size > 1024*1024 {
		return nil, errors.New("metadata exceeds 1 MiB")
	}
	encoded, err := payload.Marshal()
	if err != nil {
		return nil, err
	}
	return &Metadata{encoded: encoded}, nil
}

func (m *Metadata) Payload() (amf0.Data, error) { return amf0.Unmarshal(m.encoded) }
