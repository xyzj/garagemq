package amqp

import (
	"bytes"
	"sync/atomic"
)

type Table map[string]interface{}

// Decimal represents amqp-decimal data
type Decimal struct {
	Scale uint8
	Value int32
}

// Frame is raw frame
type Frame struct {
	Type       byte
	ChannelID  uint16
	Payload    []byte
	CloseAfter bool
}

// ContentHeader represents amqp-message content-header
type ContentHeader struct {
	ClassID       uint16
	Weight        uint16
	BodySize      uint64
	propertyFlags uint16
	PropertyList  *BasicPropertyList
}

type ConfirmMeta struct {
	ChanID           uint16
	ConnID           uint64
	DeliveryTag      uint64
	ExpectedConfirms int
	ActualConfirms   int
}

func (meta *ConfirmMeta) IsConfirmable() bool {
	return meta.ActualConfirms == meta.ExpectedConfirms
}

type Message struct {
	ID            uint64
	Header        *ContentHeader
	Exchange      string
	RoutingKey    string
	Mandatory     bool
	Immediate     bool
	BodySize      uint64
	Body          []*Frame
	DeliveryCount uint32
	ConfirmMeta   ConfirmMeta
}

var msgId uint64

func NewMessage(method *BasicPublish) *Message {
	return &Message{
		ID:            atomic.AddUint64(&msgId, 1),
		Exchange:      method.Exchange,
		RoutingKey:    method.RoutingKey,
		Mandatory:     method.Mandatory,
		Immediate:     method.Immediate,
		BodySize:      0,
		DeliveryCount: 0,
	}
}

func (message *Message) IsPersistent() bool {
	deliveryMode := message.Header.PropertyList.DeliveryMode
	return deliveryMode != nil && *deliveryMode == 2
}

func (message *Message) Append(body *Frame) {
	message.Body = append(message.Body, body)
	message.BodySize += uint64(len(body.Payload))
}

func (message *Message) Marshal(protoVersion string) (data []byte, err error) {
	buffer := bytes.NewBuffer([]byte{})
	if err = WriteLonglong(buffer, message.ID); err != nil {
		return nil, err
	}

	if err = WriteContentHeader(buffer, message.Header, protoVersion); err != nil {
		return nil, err
	}
	if err = WriteShortstr(buffer, message.Exchange); err != nil {
		return nil, err
	}
	if err = WriteShortstr(buffer, message.RoutingKey); err != nil {
		return nil, err
	}
	if err = WriteLonglong(buffer, message.BodySize); err != nil {
		return nil, err
	}

	body := bytes.NewBuffer([]byte{})
	for _, frame := range message.Body {
		if err = WriteFrame(body, frame); err != nil {
			return nil, err
		}
	}
	WriteLongstr(buffer, body.Bytes())

	if err = WriteLong(buffer, message.DeliveryCount); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (message *Message) Unmarshal(buffer []byte, protoVersion string) (err error) {
	reader := bytes.NewReader(buffer)
	if message.ID, err = ReadLonglong(reader); err != nil {
		return err
	}

	if message.Header, err = ReadContentHeader(reader, protoVersion); err != nil {
		return err
	}
	if message.Exchange, err = ReadShortstr(reader); err != nil {
		return err
	}
	if message.RoutingKey, err = ReadShortstr(reader); err != nil {
		return err
	}
	if message.BodySize, err = ReadLonglong(reader); err != nil {
		return err
	}

	rawBody, err := ReadLongstr(reader)
	bodyBuffer := bytes.NewReader(rawBody)

	for bodyBuffer.Len() != 0 {
		body, _ := ReadFrame(bodyBuffer)
		message.Body = append(message.Body, body)
	}

	if message.DeliveryCount, err = ReadLong(reader); err != nil {
		return err
	}
	return nil
}

const (
	ErrorOnConnection = iota
	ErrorOnChannel
)

type Error struct {
	ReplyCode uint16
	ReplyText string
	ClassId   uint16
	MethodId  uint16
	ErrorType int
}

func NewConnectionError(code uint16, text string, classId uint16, methodId uint16) *Error {
	err := &Error{
		ReplyCode: code,
		ReplyText: ConstantsNameMap[code] + " - " + text,
		ClassId:   classId,
		MethodId:  methodId,
		ErrorType: ErrorOnConnection,
	}

	return err
}

func NewChannelError(code uint16, text string, classId uint16, methodId uint16) *Error {
	err := &Error{
		ReplyCode: code,
		ReplyText: ConstantsNameMap[code] + " - " + text,
		ClassId:   classId,
		MethodId:  methodId,
		ErrorType: ErrorOnChannel,
	}

	return err
}
