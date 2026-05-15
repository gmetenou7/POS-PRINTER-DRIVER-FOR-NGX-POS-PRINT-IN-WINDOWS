package printers

import "time"

type Channel string

const (
	ChannelWinspool  Channel = "winspool"
	ChannelLibUSB    Channel = "libusb"
	ChannelNetwork   Channel = "network"
	ChannelSerial    Channel = "serial"
	ChannelBluetooth Channel = "bluetooth"
)

type Status string

const (
	StatusReady    Status = "ready"
	StatusPrinting Status = "printing"
	StatusOffline  Status = "offline"
	StatusError    Status = "error"
	StatusPaused   Status = "paused"
	StatusUnknown  Status = "unknown"
)

type Printer struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Channel    Channel   `json:"channel"`
	Port       string    `json:"port,omitempty"`
	Driver     string    `json:"driver,omitempty"`
	Vendor     string    `json:"vendor,omitempty"`
	Model      string    `json:"model,omitempty"`
	VID        string    `json:"vid,omitempty"`
	PID        string    `json:"pid,omitempty"`
	IsThermal  bool      `json:"isThermal"`
	IsDefault  bool      `json:"isDefault"`
	Status     Status    `json:"status"`
	DetectedAt time.Time `json:"detectedAt"`
}

type PrintJob struct {
	PrinterID string `json:"printerId,omitempty"`
	Raw       []byte `json:"-"`
	RawBase64 string `json:"raw,omitempty"`
	Text      string `json:"text,omitempty"`
	CopyCount int    `json:"copies,omitempty"`
	OpenDrawer bool  `json:"openDrawer,omitempty"`
}

type PrintResult struct {
	OK       bool   `json:"ok"`
	JobID    string `json:"jobId,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
	Duration int64  `json:"durationMs,omitempty"`
	Error    string `json:"error,omitempty"`
}
