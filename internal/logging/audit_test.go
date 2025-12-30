package logging

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestLogAuditEvent(t *testing.T) {
	// Better way: Implement a minimal LogSink using a struct
	data := &sinkData{}
	sink := &capturingSink{data: data}
	logger := logr.New(sink)

	fields := map[string]string{
		"user":   "admin",
		"action": "delete",
	}

	LogAuditEvent(logger, "resource_deletion", fields)

	// Verify
	assert.Equal(t, "Operator audit event", data.msg)

	// Check KVs
	// LogAuditEvent adds: "audit"="true", "event_type"="resource_deletion"
	// And then the fields.

	kvMap := make(map[string]interface{})
	for i := 0; i < len(data.keysAndValues); i += 2 {
		k, ok := data.keysAndValues[i].(string)
		if ok && i+1 < len(data.keysAndValues) {
			kvMap[k] = data.keysAndValues[i+1]
		}
	}

	assert.Equal(t, "true", kvMap["audit"])
	assert.Equal(t, "resource_deletion", kvMap["event_type"])
	assert.Equal(t, "admin", kvMap["user"])
	assert.Equal(t, "delete", kvMap["action"])
}

type sinkData struct {
	msg           string
	keysAndValues []interface{}
}

// capturingSink implements logr.LogSink
type capturingSink struct {
	data     *sinkData
	localKVs []interface{}
}

func (s *capturingSink) Init(info logr.RuntimeInfo) {}
func (s *capturingSink) Enabled(level int) bool     { return true }
func (s *capturingSink) Info(level int, msg string, keysAndValues ...interface{}) {
	s.data.msg = msg
	// combine local KVs with call KVs
	allKVs := append([]interface{}{}, s.localKVs...)
	allKVs = append(allKVs, keysAndValues...)
	s.data.keysAndValues = allKVs
}
func (s *capturingSink) Error(err error, msg string, keysAndValues ...interface{}) {
	s.data.msg = msg
	allKVs := append([]interface{}{}, s.localKVs...)
	allKVs = append(allKVs, keysAndValues...)
	s.data.keysAndValues = allKVs
}
func (s *capturingSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &capturingSink{
		data:     s.data,
		localKVs: append(s.localKVs, keysAndValues...),
	}
}
func (s *capturingSink) WithName(name string) logr.LogSink {
	return s
}
