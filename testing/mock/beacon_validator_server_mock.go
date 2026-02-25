package mock

import (
	"context"
	"reflect"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	gomock "go.uber.org/mock/gomock"
)

// MockBlockProposer is a mock of the BlockProposer interface
// (beacon-chain/rpc/eth/beacon.BlockProposer).
type MockBlockProposer struct {
	ctrl     *gomock.Controller
	recorder *MockBlockProposerMockRecorder
}

// MockBlockProposerMockRecorder is the mock recorder for MockBlockProposer.
type MockBlockProposerMockRecorder struct {
	mock *MockBlockProposer
}

// NewMockBlockProposer creates a new mock instance.
func NewMockBlockProposer(ctrl *gomock.Controller) *MockBlockProposer {
	mock := &MockBlockProposer{ctrl: ctrl}
	mock.recorder = &MockBlockProposerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBlockProposer) EXPECT() *MockBlockProposerMockRecorder {
	return m.recorder
}

// ProposeBeaconBlock mocks base method.
func (m *MockBlockProposer) ProposeBeaconBlock(ctx context.Context, blk *eth.GenericSignedBeaconBlock) (*eth.ProposeResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProposeBeaconBlock", ctx, blk)
	ret0, _ := ret[0].(*eth.ProposeResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ProposeBeaconBlock indicates an expected call of ProposeBeaconBlock.
func (mr *MockBlockProposerMockRecorder) ProposeBeaconBlock(ctx, blk any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProposeBeaconBlock", reflect.TypeFor[func(ctx context.Context, blk *eth.GenericSignedBeaconBlock) (*eth.ProposeResponse, error)](), ctx, blk)
}
