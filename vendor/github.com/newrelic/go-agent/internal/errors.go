package internal

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/newrelic/go-agent/internal/jsonx"
)

const (
	// PanicErrorKlass is the error klass used for errors generated by
	// recovering panics in txn.End.
	PanicErrorKlass = "panic"
)

func panicValueMsg(v interface{}) string {
	switch val := v.(type) {
	case error:
		return val.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// TxnErrorFromPanic creates a new TxnError from a panic.
func TxnErrorFromPanic(now time.Time, v interface{}) ErrorData {
	return ErrorData{
		When:  now,
		Msg:   panicValueMsg(v),
		Klass: PanicErrorKlass,
	}
}

// TxnErrorFromResponseCode creates a new TxnError from an http response code.
func TxnErrorFromResponseCode(now time.Time, code int) ErrorData {
	return ErrorData{
		When:  now,
		Msg:   http.StatusText(code),
		Klass: strconv.Itoa(code),
	}
}

// ErrorData contains the information about a recorded error.
type ErrorData struct {
	When            time.Time
	Stack           StackTrace
	ExtraAttributes map[string]interface{}
	Msg             string
	Klass           string
}

// TxnError combines error data with information about a transaction.  TxnError is used for
// both error events and traced errors.
type TxnError struct {
	ErrorData
	TxnEvent
}

// ErrorEvent and tracedError are separate types so that error events and traced errors can have
// different WriteJSON methods.
type ErrorEvent TxnError

type tracedError TxnError

// TxnErrors is a set of errors captured in a Transaction.
type TxnErrors []*ErrorData

// NewTxnErrors returns a new empty TxnErrors.
func NewTxnErrors(max int) TxnErrors {
	return make([]*ErrorData, 0, max)
}

// Add adds a TxnError.
func (errors *TxnErrors) Add(e ErrorData) {
	if len(*errors) < cap(*errors) {
		*errors = append(*errors, &e)
	}
}

func (h *tracedError) WriteJSON(buf *bytes.Buffer) {
	buf.WriteByte('[')
	jsonx.AppendFloat(buf, timeToFloatMilliseconds(h.When))
	buf.WriteByte(',')
	jsonx.AppendString(buf, h.FinalName)
	buf.WriteByte(',')
	jsonx.AppendString(buf, h.Msg)
	buf.WriteByte(',')
	jsonx.AppendString(buf, h.Klass)
	buf.WriteByte(',')

	buf.WriteByte('{')
	buf.WriteString(`"agentAttributes"`)
	buf.WriteByte(':')
	agentAttributesJSON(h.Attrs, buf, destError)
	buf.WriteByte(',')
	buf.WriteString(`"userAttributes"`)
	buf.WriteByte(':')
	userAttributesJSON(h.Attrs, buf, destError, h.ErrorData.ExtraAttributes)
	buf.WriteByte(',')
	buf.WriteString(`"intrinsics"`)
	buf.WriteByte(':')
	intrinsicsJSON(&h.TxnEvent, buf)
	if nil != h.Stack {
		buf.WriteByte(',')
		buf.WriteString(`"stack_trace"`)
		buf.WriteByte(':')
		h.Stack.WriteJSON(buf)
	}
	buf.WriteByte('}')

	buf.WriteByte(']')
}

// MarshalJSON is used for testing.
func (h *tracedError) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	h.WriteJSON(buf)
	return buf.Bytes(), nil
}

type harvestErrors []*tracedError

func newHarvestErrors(max int) harvestErrors {
	return make([]*tracedError, 0, max)
}

// MergeTxnErrors merges a transaction's errors into the harvest's errors.
func MergeTxnErrors(errors *harvestErrors, errs TxnErrors, txnEvent TxnEvent) {
	for _, e := range errs {
		if len(*errors) == cap(*errors) {
			return
		}
		*errors = append(*errors, &tracedError{
			TxnEvent:  txnEvent,
			ErrorData: *e,
		})
	}
}

func (errors harvestErrors) Data(agentRunID string, harvestStart time.Time) ([]byte, error) {
	if 0 == len(errors) {
		return nil, nil
	}
	estimate := 1024 * len(errors)
	buf := bytes.NewBuffer(make([]byte, 0, estimate))
	buf.WriteByte('[')
	jsonx.AppendString(buf, agentRunID)
	buf.WriteByte(',')
	buf.WriteByte('[')
	for i, e := range errors {
		if i > 0 {
			buf.WriteByte(',')
		}
		e.WriteJSON(buf)
	}
	buf.WriteByte(']')
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func (errors harvestErrors) MergeIntoHarvest(h *Harvest) {}

func (errors harvestErrors) EndpointMethod() string {
	return cmdErrorData
}
