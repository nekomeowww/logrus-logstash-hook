package logrustash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simpleFmter struct{}

func (f simpleFmter) Format(e *logrus.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("msg: %#v", e.Message)), nil
}

func TestFire(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	buffer := bytes.NewBuffer(nil)
	h := Hook{
		writer:    buffer,
		formatter: simpleFmter{},
	}

	entry := &logrus.Entry{
		Message: "my message",
		Data:    logrus.Fields{},
	}

	err := h.Fire(entry)
	require.NoError(err)

	expected := "msg: \"my message\""
	assert.Equal(expected, buffer.String())
}

type FailFmt struct{}

func (f FailFmt) Format(e *logrus.Entry) ([]byte, error) {
	return nil, errors.New("")
}

func TestFireFormatError(t *testing.T) {
	assert := assert.New(t)

	buffer := bytes.NewBuffer(nil)
	h := Hook{
		writer:    buffer,
		formatter: FailFmt{},
	}

	err := h.Fire(&logrus.Entry{Data: logrus.Fields{}})
	assert.Error(err)
}

type FailWrite struct{}

func (w FailWrite) Write(d []byte) (int, error) {
	return 0, errors.New("")
}

func TestFireWriteError(t *testing.T) {
	assert := assert.New(t)

	h := Hook{
		writer:    FailWrite{},
		formatter: &logrus.JSONFormatter{},
	}

	err := h.Fire(&logrus.Entry{Data: logrus.Fields{}})
	assert.Error(err)
}

func TestDefaultFormatterWithFields(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	format := DefaultFormatter(logrus.Fields{"ID": 123})

	entry := &logrus.Entry{
		Message: "msg1",
		Data:    logrus.Fields{"f1": "bla"},
	}

	res, err := format.Format(entry)
	require.NoError(err)
	require.NotEmpty(res)

	expected := []string{
		"fields\":\"f1=bla",
		"ID\":123",
		"message\":\"msg1\"",
	}

	for _, exp := range expected {
		assert.Contains(string(res), exp)
	}
}

func TestDefaultFormatterWithCaller(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	format := DefaultFormatter(logrus.Fields{"ID": 123})

	entry := &logrus.Entry{
		Message: "msg1",
		Logger:  logrus.New(),
	}

	pc, f, l, _ := runtime.Caller(0)
	functionName := runtime.FuncForPC(pc).Name()
	entry.Logger.ReportCaller = true
	entry.Context = context.WithValue(context.Background(), ContextKeyRuntimeCaller, &runtime.Frame{
		File:     f,
		Line:     l,
		Function: functionName,
	})

	res, err := format.Format(entry)
	require.NoError(err)
	require.NotEmpty(res)

	expected := []string{
		fmt.Sprintf("file\":\"%s:%d", f, l),
		fmt.Sprintf("function\":\"%s", functionName),
		"ID\":123",
		"message\":\"msg1\"",
	}

	for _, exp := range expected {
		assert.Contains(string(res), exp)
	}
}

func TestDefaultFormatterWithEmptyFields(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	now := time.Now()
	formatter := DefaultFormatter(logrus.Fields{})

	entry := &logrus.Entry{
		Message: "message bla bla",
		Level:   logrus.DebugLevel,
		Time:    now,
		Data: logrus.Fields{
			"Key1": "Value1",
		},
	}

	res, err := formatter.Format(entry)
	require.NoError(err)

	expected := []string{
		"\"message\":\"message bla bla\"",
		"\"level\":\"debug\"",
		"\"fields\":\"Key1=Value1\"",
		"\"@version\":\"1\"",
		"\"type\":\"log\"",
		fmt.Sprintf("\"@timestamp\":\"%s\"", now.Format(time.RFC3339)),
	}

	for _, exp := range expected {
		assert.Contains(string(res), exp)
	}
}

func TestLogstashFieldsNotOverridden(t *testing.T) {
	assert := assert.New(t)

	_ = DefaultFormatter(logrus.Fields{"user1": "11"})

	_, ok := logstashFields["user1"]
	assert.False(ok)
}
