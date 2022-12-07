package logrustash

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

const (
	defaultLogrusEntryFireChannelBufferSize = 8192
)

type ContextKey string

const (
	ContextKeyRuntimeCaller ContextKey = "context.key.runtime.caller"
)

// Hook represents a Logstash hook.
// It has two fields: writer to write the entry to Logstash and
// formatter to format the entry to a Logstash format before sending.
//
// To initialize it use the `New` function.
type Hook struct {
	sync.RWMutex

	conn                   io.Writer
	protocol               string
	addr                   string
	logrusEntryFireChannel chan *logrus.Entry
	formatter              logrus.Formatter
}

type HookOptions struct {
	// KeepAlive enables TCP keepalive.
	KeepAlive bool
	// KeepAlivePeriod sets the TCP keepalive period.
	KeepAlivePeriod time.Duration
	// FireChannelBufferSize sets the size of the logrus entry fire channel.
	FireChannelBufferSize int
}

// GetKeepAlivePeriod returns the keep alive period, defaults to 30 seconds.
func (h HookOptions) GetKeepAlivePeriod() time.Duration {
	if h.KeepAlivePeriod > 0 {
		return h.KeepAlivePeriod
	}

	return time.Second * 30
}

// GetFireChannelBufferSize returns the fire channel buffer size, defaults to 8192.
func (h HookOptions) GetFireChannelBufferSize() int {
	if h.FireChannelBufferSize > 0 {
		return h.FireChannelBufferSize
	}

	return defaultLogrusEntryFireChannelBufferSize
}

// New returns a new logrus.Hook for Logstash
func New(protocol, addr string, f logrus.Formatter, opts ...HookOptions) (logrus.Hook, error) {
	if protocol == "" || addr == "" {
		return nil, fmt.Errorf("protocol and addr must be set")
	}

	// dial the connection
	conn, err := net.Dial(protocol, addr)
	if err != nil {
		return nil, err
	}

	h := &Hook{
		protocol:  protocol,
		addr:      addr,
		conn:      conn,
		formatter: f,
	}
	// apply options
	if len(opts) > 0 {
		opt := opts[0]
		// apply keep alive options
		if opt.KeepAlive {
			if c, ok := conn.(*net.TCPConn); ok && c != nil {
				err = c.SetKeepAlive(true)
				if err != nil {
					return nil, err
				}

				err = c.SetKeepAlivePeriod(opt.GetKeepAlivePeriod())
				if err != nil {
					return nil, err
				}
			}
		}

		// apply fire channel buffer size
		h.logrusEntryFireChannel = make(chan *logrus.Entry, opt.GetFireChannelBufferSize())
	}

	// if fire channel is not set, create a default one
	if h.logrusEntryFireChannel == nil {
		h.logrusEntryFireChannel = make(chan *logrus.Entry, defaultLogrusEntryFireChannelBufferSize)
	}

	// split a goroutine to handle logrus entry fire channel
	go func() {
		// defer recover
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "panic in logrus entry fire channel: %v\n", r)
				debug.PrintStack()
			}
		}()

		// handle logrus entry fire channel
		for e := range h.logrusEntryFireChannel {
			if err := h.fire(e); err != nil {
				fmt.Fprintf(os.Stderr, "failed to send log to logstash, error: %v\n", err)
			}
		}
	}()

	return h, nil
}

// reconnect reconnects to the logstash server.
func (h *Hook) reconnect() {
	fmt.Fprintln(os.Stderr, "failed to send log entry to logstash, reconnecting...")

	// Sleep before reconnect.
	_, _, _ = lo.AttemptWithDelay(0, time.Second*5, func(index int, duration time.Duration) error {
		conn, err := net.Dial(h.protocol, h.addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to reconnect to logstash, error: %s (current attempt %d)\n", err, index+1)
			return err
		}

		h.Lock()
		h.conn = conn
		h.Unlock()
		return nil
	})
}

// processSendError processes the error returned by the send function.
func (h *Hook) processSendError(err error, data []byte) error {
	netErr, ok := err.(net.Error)
	if !ok {
		// return if its not net.Error
		return err
	}

	// if its a timeout error, try to resend the data
	if netErr.Timeout() {
		fmt.Fprintf(os.Stderr, "failed to send log entry to logstash, error: %s, resending...\n", err)
		return h.send(data)
	}

	// otherwise reconnect and try to resend the data
	h.reconnect()
	return h.send(data)
}

// send sends the data to the logstash server.
func (h *Hook) send(data []byte) error {
	h.Lock()
	_, err := h.conn.Write(data)
	h.Unlock()
	if err != nil {
		return h.processSendError(err, data)
	}

	return nil
}

// fire wraps the fire function to handle the logrus entry fire channel.
func (h *Hook) fire(e *logrus.Entry) error {
	dataBytes, err := h.formatter.Format(e)
	if err != nil {
		return err
	}

	err = h.send(dataBytes)
	return err
}

// Fire takes, formats and sends the entry to Logstash.
// Hook's formatter is used to format the entry into Logstash format
// and Hook's writer is used to write the formatted entry to the Logstash instance.
func (h *Hook) Fire(e *logrus.Entry) error {
	if h.logrusEntryFireChannel != nil {
		h.logrusEntryFireChannel <- e
		return nil
	} else {
		fmt.Fprintln(os.Stderr, "logrus entry fire channel is not initialized or closed")
	}

	return h.fire(e)
}

// Levels returns all logrus levels.
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Using a pool to re-use of old entries when formatting Logstash messages.
// It is used in the Fire function.
var entryPool = sync.Pool{
	New: func() interface{} {
		return &logrus.Entry{}
	},
}

// copyEntry copies the entry `e` to a new entry and then adds all the fields in `fields` that are missing in the new entry data.
// It uses `entryPool` to re-use allocated entries.
func copyEntry(e *logrus.Entry, fields logrus.Fields) *logrus.Entry {
	ne := entryPool.Get().(*logrus.Entry)
	ne.Message = e.Message
	ne.Level = e.Level
	ne.Time = e.Time
	ne.Data = logrus.Fields{}

	if e.Logger.ReportCaller && e.Context != nil {
		caller, _ := e.Context.Value(ContextKeyRuntimeCaller).(*runtime.Frame)
		if caller != nil {
			ne.Data["function"] = caller.Function
			ne.Data["file"] = fmt.Sprintf("%s:%d", caller.File, caller.Line)
		}
	}

	if e.Logger.ReportCaller && e.Data["file"] != nil {
		ne.Data["file"] = e.Data["file"]
		delete(e.Data, "file")
	}
	if e.Logger.ReportCaller && e.Data["function"] != nil {
		ne.Data["function"] = e.Data["function"]
		delete(e.Data, "function")
	}

	if len(e.Data) > 0 {
		fieldsStrings := make([]string, 0)
		for k, v := range e.Data {
			fieldsStrings = append(fieldsStrings, fmt.Sprintf("%s=%v", k, v))
			delete(e.Data, k)
		}
		ne.Data["fields"] = strings.Join(fieldsStrings, " ")
	}

	for k, v := range fields {
		ne.Data[k] = v
	}

	return ne
}

// releaseEntry puts the given entry back to `entryPool`. It must be called if copyEntry is called.
func releaseEntry(e *logrus.Entry) {
	entryPool.Put(e)
}

// LogstashFormatter represents a Logstash format.
// It has logrus.Formatter which formats the entry and logrus.Fields which
// are added to the JSON message if not given in the entry data.
//
// Note: use the `DefaultFormatter` function to set a default Logstash formatter.
type LogstashFormatter struct {
	logrus.Formatter
	logrus.Fields
}

var (
	logstashFields   = logrus.Fields{"@version": "1", "type": "log"}
	logstashFieldMap = logrus.FieldMap{
		logrus.FieldKeyTime: "@timestamp",
		logrus.FieldKeyMsg:  "message",
	}
)

// DefaultFormatter returns a default Logstash formatter:
// A JSON format with "@version" set to "1" (unless set differently in `fields`,
// "type" to "log" (unless set differently in `fields`),
// "@timestamp" to the log time and "message" to the log message.
//
// Note: to set a different configuration use the `LogstashFormatter` structure.
func DefaultFormatter(fields logrus.Fields) logrus.Formatter {
	for k, v := range logstashFields {
		if _, ok := fields[k]; !ok {
			fields[k] = v
		}
	}

	return LogstashFormatter{
		Formatter: &logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap:        logstashFieldMap,
		},
		Fields: fields,
	}
}

// Format formats an entry to a Logstash format according to the given Formatter and Fields.
//
// Note: the given entry is copied and not changed during the formatting process.
func (f LogstashFormatter) Format(e *logrus.Entry) ([]byte, error) {
	ne := copyEntry(e, f.Fields)
	dataBytes, err := f.Formatter.Format(ne)
	releaseEntry(ne)
	return dataBytes, err
}
