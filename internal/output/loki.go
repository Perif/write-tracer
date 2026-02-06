package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"write-tracer/internal/event"
)

type LokiClient struct {
	endpoint string
	client   *http.Client
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

func NewLokiClient(endpoint string) *LokiClient {
	return &LokiClient{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (l *LokiClient) Push(ev event.WriteEvent) error {

	stream := lokiStream{
		Stream: map[string]string{
			"app":  "write-tracer",
			"pid":  fmt.Sprintf("%d", ev.PID),
			"comm": ev.CommString(),
			"fd":   fmt.Sprintf("%d", ev.FD),
		},
		Values: [][]string{
			{fmt.Sprintf("%d", time.Now().UnixNano()), ev.DataString()},
		},
	}

	req := lokiPushRequest{Streams: []lokiStream{stream}}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// DEBUG: Get the loki push request body
	// slog.Info("Loki push request", "body", string(body))

	resp, err := l.client.Post(l.endpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// DEBUG: Read response body to see what Loki is complaining about
		respBody, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(respBody))
		// return fmt.Errorf("loki returned status %d", resp.StatusCode)
	}

	return nil
}
