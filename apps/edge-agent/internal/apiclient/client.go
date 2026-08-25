// Package apiclient talks to the central API. The agent only ever makes
// outbound HTTPS calls to it — this is what lets each site sit behind its
// own ISP/NAT with zero inbound port exposure.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type Camera struct {
	ID                       string         `json:"id"`
	Name                     string         `json:"name"`
	EzvizSerial              string         `json:"ezviz_serial"`
	LocalRTSPURL             string         `json:"local_rtsp_url"`
	LocalRTSPURLSub          string         `json:"local_rtsp_url_sub"`
	ChannelNo                int            `json:"channel_no"`
	Status                   string         `json:"status"`
	RecordingStorageTargetID *string        `json:"recording_storage_target_id"`
	StorageTarget            *StorageTarget `json:"storage_target"`
}

type StorageTarget struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

type HeartbeatResponse struct {
	SiteID   string   `json:"site_id"`
	SiteName string   `json:"site_name"`
	Cameras  []Camera `json:"cameras"`
}

func (c *Client) Heartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	var resp HeartbeatResponse
	if err := c.do(ctx, http.MethodPost, "/api/agent/heartbeat", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ReportCameraStatus(ctx context.Context, cameraID, status string) error {
	body := map[string]string{"camera_id": cameraID, "status": status}
	return c.do(ctx, http.MethodPost, "/api/agent/camera-status", body, nil)
}

type ReportRecordingRequest struct {
	CameraID        string `json:"camera_id"`
	StorageTargetID string `json:"storage_target_id"`
	ObjectKey       string `json:"object_key"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	SizeBytes       int64  `json:"size_bytes"`
}

func (c *Client) ReportRecording(ctx context.Context, req ReportRecordingRequest) error {
	return c.do(ctx, http.MethodPost, "/api/agent/recordings", req, nil)
}

func (c *Client) ReportUploadFailure(ctx context.Context, cameraID, errMsg string) error {
	body := map[string]string{"camera_id": cameraID, "error": errMsg}
	return c.do(ctx, http.MethodPost, "/api/agent/upload-failure", body, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Agent "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
