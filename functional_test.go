package logrustash

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntryIsNotChangedByLogstashFormatter(t *testing.T) {
	assert := assert.New(t)

	buffer := bytes.NewBufferString("")
	bufferOut := bytes.NewBufferString("")

	log := logrus.New()
	log.Out = bufferOut

	l, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:8989")))
	require.NoError(t, err)
	defer l.Close()

	hook, err := New("tcp", "127.0.0.1:8989", DefaultFormatter(logrus.Fields{"NICKNAME": ""}))
	require.NoError(t, err)
	hook.(*Hook).conn = buffer

	log.Hooks.Add(hook)
	log.Info("hello world")

	assert.Contains(buffer.String(), `NICKNAME":`, fmt.Sprintf("expected logstash message to have '%s': %v", `NICKNAME":`, buffer.String()))
	assert.NotContains(bufferOut.String(), `NICKNAME":`, fmt.Sprintf("expected main logrus message to not have '%s': %v", `NICKNAME":`, buffer.String()))
}

func TestTimestampFormatKitchen(t *testing.T) {
	assert := assert.New(t)

	log := logrus.New()
	buffer := bytes.NewBufferString("")

	l, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:8989")))
	require.NoError(t, err)
	defer l.Close()

	hook, err := New("tcp", "127.0.0.1:8989", LogstashFormatter{
		Formatter: &logrus.JSONFormatter{
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "@timestamp",
				logrus.FieldKeyMsg:  "message",
			},
			TimestampFormat: time.Kitchen,
		},
		Fields: logrus.Fields{"HOSTNAME": "localhost", "USERNAME": "root"},
	})
	require.NoError(t, err)
	hook.(*Hook).conn = buffer

	log.Hooks.Add(hook)
	log.Error("this is an error message!")

	mTime := time.Now()
	expected := fmt.Sprintf(`{"@timestamp":"%s","HOSTNAME":"localhost","USERNAME":"root","level":"error","message":"this is an error message!"}`+"\n", mTime.Format(time.Kitchen))
	assert.Equal(expected, buffer.String(), fmt.Sprintf("expected JSON to be '%#v' but got '%#v'", expected, buffer.String()))
}

func TestTextFormatLogstash(t *testing.T) {
	assert := assert.New(t)

	log := logrus.New()
	buffer := bytes.NewBufferString("")

	l, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:8989")))
	require.NoError(t, err)
	defer l.Close()

	hook, err := New("tcp", "127.0.0.1:8989", LogstashFormatter{
		Formatter: &logrus.TextFormatter{
			TimestampFormat: time.Kitchen,
		},
		Fields: logrus.Fields{"HOSTNAME": "localhost", "USERNAME": "root"},
	})
	require.NoError(t, err)
	hook.(*Hook).conn = buffer

	log.Hooks.Add(hook)
	log.Warning("this is a warning message!")

	mTime := time.Now()
	expected := fmt.Sprintf(`time="%s" level=warning msg="this is a warning message!" HOSTNAME=localhost USERNAME=root
`, mTime.Format(time.Kitchen))
	assert.Equal(expected, buffer.String(), fmt.Sprintf("expected JSON to be '%#v' but got '%v'", expected, buffer.String()))
}

// Github issue #39
func TestLogWithFieldsDoesNotOverrideHookFields(t *testing.T) {
	assert := assert.New(t)

	log := logrus.New()
	buffer := bytes.NewBufferString("")

	l, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:8989")))
	require.NoError(t, err)
	defer l.Close()

	hook, err := New("tcp", "127.0.0.1:8989", LogstashFormatter{
		Formatter: &logrus.JSONFormatter{},
		Fields:    logrus.Fields{},
	})
	require.NoError(t, err)
	hook.(*Hook).conn = buffer

	log.Hooks.Add(hook)
	log.WithField("animal", "walrus").Info("bla")

	attr := `fields":"animal=walrus`
	assert.Contains(buffer.String(), attr, fmt.Sprintf("expected to have '%s' in '%s'", attr, buffer.String()))

	buffer.Reset()
	log.Info("hahaha")
	assert.NotContains(buffer.String(), attr, fmt.Sprintf("expected not to have '%s' in '%s'", attr, buffer.String()))
}

func TestDefaultFormatterNotOverrideMyLogstashFieldsValues(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	formatter := DefaultFormatter(logrus.Fields{"@version": "2", "type": "mylogs"})

	dataBytes, err := formatter.Format(&logrus.Entry{Data: logrus.Fields{}})
	require.NoError(err, fmt.Sprintf("expected Format to not return error: %s", err))

	expected := []string{
		`"@version":"2"`,
		`"type":"mylogs"`,
	}

	for _, expField := range expected {
		assert.Contains(string(dataBytes), expField, "expected '%s' to be in '%s'", expField, string(dataBytes))
	}
}

func TestDefaultFormatterLogstashFields(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	formatter := DefaultFormatter(logrus.Fields{})

	dataBytes, err := formatter.Format(&logrus.Entry{Data: logrus.Fields{}})
	require.NoError(err, "expected Format to not return error: %s", err)

	expected := []string{
		`"@version":"1"`,
		`"type":"log"`,
	}

	for _, expField := range expected {
		assert.Contains(string(dataBytes), expField, "expected '%s' to be in '%s'", expField, string(dataBytes))
	}
}

// UDP will never fail because it's connectionless.
// That's why I am using it for this integration tests just to make sure
// it won't fail when a data is written.
func TestUDPWritter(t *testing.T) {
	log := logrus.New()
	hook, err := New("udp", ":8282", &logrus.JSONFormatter{})
	require.NoError(t, err)

	log.Hooks.Add(hook)
	log.Info("this is an information message")
}
