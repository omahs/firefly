// Code generated by mockery v1.0.0. DO NOT EDIT.

package namespacemocks

import (
	context "context"

	fftypes "github.com/hyperledger/firefly-common/pkg/fftypes"
	core "github.com/hyperledger/firefly/pkg/core"

	mock "github.com/stretchr/testify/mock"

	orchestrator "github.com/hyperledger/firefly/internal/orchestrator"

	spievents "github.com/hyperledger/firefly/internal/spievents"
)

// Manager is an autogenerated mock type for the Manager type
type Manager struct {
	mock.Mock
}

// Authorize provides a mock function with given fields: ctx, authReq
func (_m *Manager) Authorize(ctx context.Context, authReq *fftypes.AuthReq) error {
	ret := _m.Called(ctx, authReq)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *fftypes.AuthReq) error); ok {
		r0 = rf(ctx, authReq)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetNamespaces provides a mock function with given fields: ctx
func (_m *Manager) GetNamespaces(ctx context.Context) ([]*core.Namespace, error) {
	ret := _m.Called(ctx)

	var r0 []*core.Namespace
	if rf, ok := ret.Get(0).(func(context.Context) []*core.Namespace); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*core.Namespace)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetOperationByNamespacedID provides a mock function with given fields: ctx, nsOpID
func (_m *Manager) GetOperationByNamespacedID(ctx context.Context, nsOpID string) (*core.Operation, error) {
	ret := _m.Called(ctx, nsOpID)

	var r0 *core.Operation
	if rf, ok := ret.Get(0).(func(context.Context, string) *core.Operation); ok {
		r0 = rf(ctx, nsOpID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*core.Operation)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, nsOpID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Init provides a mock function with given fields: ctx, cancelCtx, reset
func (_m *Manager) Init(ctx context.Context, cancelCtx context.CancelFunc, reset chan bool) error {
	ret := _m.Called(ctx, cancelCtx, reset)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, context.CancelFunc, chan bool) error); ok {
		r0 = rf(ctx, cancelCtx, reset)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Orchestrator provides a mock function with given fields: ns
func (_m *Manager) Orchestrator(ns string) orchestrator.Orchestrator {
	ret := _m.Called(ns)

	var r0 orchestrator.Orchestrator
	if rf, ok := ret.Get(0).(func(string) orchestrator.Orchestrator); ok {
		r0 = rf(ns)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(orchestrator.Orchestrator)
		}
	}

	return r0
}

// Reset provides a mock function with given fields: ctx
func (_m *Manager) Reset(ctx context.Context) {
	_m.Called(ctx)
}

// ResolveOperationByNamespacedID provides a mock function with given fields: ctx, nsOpID, op
func (_m *Manager) ResolveOperationByNamespacedID(ctx context.Context, nsOpID string, op *core.OperationUpdateDTO) error {
	ret := _m.Called(ctx, nsOpID, op)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *core.OperationUpdateDTO) error); ok {
		r0 = rf(ctx, nsOpID, op)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SPIEvents provides a mock function with given fields:
func (_m *Manager) SPIEvents() spievents.Manager {
	ret := _m.Called()

	var r0 spievents.Manager
	if rf, ok := ret.Get(0).(func() spievents.Manager); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(spievents.Manager)
		}
	}

	return r0
}

// Start provides a mock function with given fields:
func (_m *Manager) Start() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// WaitStop provides a mock function with given fields:
func (_m *Manager) WaitStop() {
	_m.Called()
}