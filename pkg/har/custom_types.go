package har

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/martian/har"
)

// FlexibleHAR is a HAR that can parse time fields as either int or float
type FlexibleHAR struct {
	Log *FlexibleLog `json:"log"`
}

// FlexibleLog represents the HAR log with flexible parsing
type FlexibleLog struct {
	Version string          `json:"version"`
	Creator *har.Creator    `json:"creator"`
	Entries []FlexibleEntry `json:"entries"`
	// Additional fields that might be in HAR files but not in martian/har
	Browser interface{} `json:"browser,omitempty"`
	Pages   interface{} `json:"pages,omitempty"`
	Comment string      `json:"comment,omitempty"`
}

// FlexibleEntry allows time to be parsed as either int or float
type FlexibleEntry struct {
	ID              string            `json:"_id,omitempty"`
	StartedDateTime time.Time         `json:"startedDateTime"`
	Time            FlexibleTime      `json:"time"`
	Request         *har.Request      `json:"request"`
	Response        *FlexibleResponse `json:"response,omitempty"`
	Cache           *har.Cache        `json:"cache,omitempty"`
	Timings         *FlexibleTimings  `json:"timings,omitempty"`
	ServerIPAddress string            `json:"serverIPAddress,omitempty"`
	Connection      string            `json:"connection,omitempty"`
	Comment         string            `json:"comment,omitempty"`
}

// FlexibleTime handles both int and float JSON values
type FlexibleTime int64

// UnmarshalJSON implements custom unmarshaling for FlexibleTime
func (ft *FlexibleTime) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as float64 first (handles both int and float)
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*ft = FlexibleTime(int64(f))
	return nil
}

// FlexibleTimings allows timing fields to be parsed as either int or float
type FlexibleTimings struct {
	Send    FlexibleTime `json:"send"`
	Wait    FlexibleTime `json:"wait"`
	Receive FlexibleTime `json:"receive"`
	// Additional timing fields that might be in HAR files
	Blocked FlexibleTime `json:"blocked,omitempty"`
	DNS     FlexibleTime `json:"dns,omitempty"`
	Connect FlexibleTime `json:"connect,omitempty"`
	SSL     FlexibleTime `json:"ssl,omitempty"`
}

// ToStandardTimings converts FlexibleTimings to standard har.Timings
func (ft *FlexibleTimings) ToStandardTimings() *har.Timings {
	if ft == nil {
		return nil
	}
	return &har.Timings{
		Send:    int64(ft.Send),
		Wait:    int64(ft.Wait),
		Receive: int64(ft.Receive),
	}
}

// FlexibleResponse allows content.text to be parsed as either string or base64
type FlexibleResponse struct {
	Status      int              `json:"status"`
	StatusText  string           `json:"statusText"`
	HTTPVersion string           `json:"httpVersion"`
	Cookies     []har.Cookie     `json:"cookies,omitempty"`
	Headers     []har.Header     `json:"headers,omitempty"`
	Content     *FlexibleContent `json:"content"`
	RedirectURL string           `json:"redirectURL"`
	HeadersSize int64            `json:"headersSize"`
	BodySize    int64            `json:"bodySize"`
}

// FlexibleContent handles text field that can be either plain text or base64
type FlexibleContent struct {
	Size     int64           `json:"size"`
	MimeType string          `json:"mimeType"`
	Text     json.RawMessage `json:"text,omitempty"`
	Encoding string          `json:"encoding,omitempty"`
}

// ToStandardContent converts FlexibleContent to standard har.Content
func (fc *FlexibleContent) ToStandardContent() *har.Content {
	if fc == nil {
		return nil
	}

	content := &har.Content{
		Size:     fc.Size,
		MimeType: fc.MimeType,
		Encoding: fc.Encoding,
	}

	// Handle text field
	if len(fc.Text) > 0 {
		// Try to unmarshal as string first
		var textStr string
		if err := json.Unmarshal(fc.Text, &textStr); err == nil {
			// It's a string, convert to bytes
			if fc.Encoding == "base64" {
				// If it's marked as base64, decode it
				decoded, err := base64.StdEncoding.DecodeString(textStr)
				if err == nil {
					content.Text = decoded
				} else {
					// If base64 decode fails, use as-is
					content.Text = []byte(textStr)
				}
			} else {
				// Plain text
				content.Text = []byte(textStr)
			}
		} else {
			// Maybe it's already []byte in JSON (unlikely but handle it)
			var textBytes []byte
			if err := json.Unmarshal(fc.Text, &textBytes); err == nil {
				content.Text = textBytes
			}
		}
	}

	return content
}

// ToStandardResponse converts FlexibleResponse to standard har.Response
func (fr *FlexibleResponse) ToStandardResponse() *har.Response {
	if fr == nil {
		return nil
	}

	return &har.Response{
		Status:      fr.Status,
		StatusText:  fr.StatusText,
		HTTPVersion: fr.HTTPVersion,
		Cookies:     fr.Cookies,
		Headers:     fr.Headers,
		Content:     fr.Content.ToStandardContent(),
		RedirectURL: fr.RedirectURL,
		HeadersSize: fr.HeadersSize,
		BodySize:    fr.BodySize,
	}
}

// ToStandardHAR converts FlexibleHAR to standard har.HAR
func (fh *FlexibleHAR) ToStandardHAR() *har.HAR {
	standardHAR := &har.HAR{
		Log: &har.Log{
			Version: fh.Log.Version,
			Creator: fh.Log.Creator,
		},
	}

	// Convert flexible entries to standard entries
	standardHAR.Log.Entries = make([]*har.Entry, len(fh.Log.Entries))
	for i, flexEntry := range fh.Log.Entries {
		standardHAR.Log.Entries[i] = &har.Entry{
			ID:              flexEntry.ID,
			StartedDateTime: flexEntry.StartedDateTime,
			Time:            int64(flexEntry.Time),
			Request:         flexEntry.Request,
			Response:        flexEntry.Response.ToStandardResponse(),
			Cache:           flexEntry.Cache,
			Timings:         flexEntry.Timings.ToStandardTimings(),
		}
	}

	return standardHAR
}
