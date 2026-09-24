package tests

import (
	"webtyp.com/model"
)

type mockCaller struct {
	lastOp   string
	lastArgs model.Encodable
	onCall   func(op string, args model.Encodable, into model.Decodable) error
}

func (m *mockCaller) Call(op string, args model.Encodable, into model.Decodable, done func(err error)) {
	m.lastOp = op
	m.lastArgs = args
	if m.onCall != nil {
		err := m.onCall(op, args, into)
		done(err)
	} else {
		done(nil)
	}
}

func (m *mockCaller) Dispatch(op string, args model.Encodable) {
	m.lastOp = op
	m.lastArgs = args
}
