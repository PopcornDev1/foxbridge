package bridge

import (
	"encoding/json"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

// handleIO handles IO domain methods (used for PDF streaming).
func (b *Bridge) handleIO(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "IO.read":
		var params struct {
			Handle string `json:"handle"`
			Offset int    `json:"offset"`
			Size   int    `json:"size"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		b.pdfStreamsMu.Lock()
		data, ok := b.pdfStreams[params.Handle]
		b.pdfStreamsMu.Unlock()

		if !ok {
			return nil, &cdp.Error{Code: -32000, Message: "stream not found: " + params.Handle}
		}

		// Return all data at once (base64 encoded)
		return marshalResult(map[string]interface{}{
			"base64Encoded": true,
			"data":          data,
			"eof":           true,
		})

	case "IO.close":
		var params struct {
			Handle string `json:"handle"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		b.pdfStreamsMu.Lock()
		delete(b.pdfStreams, params.Handle)
		b.pdfStreamsMu.Unlock()

		return json.RawMessage(`{}`), nil

	default:
		return json.RawMessage(`{}`), nil
	}
}
