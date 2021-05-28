// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package events

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaleido-io/firefly/mocks/broadcastmocks"
	"github.com/kaleido-io/firefly/mocks/databasemocks"
	"github.com/kaleido-io/firefly/pkg/fftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func uuidMatches(id1 *fftypes.UUID) interface{} {
	return mock.MatchedBy(func(id2 *fftypes.UUID) bool { return id1.Equals(id2) })
}

func TestShutdownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	mdi.On("GetOffset", mock.Anything, fftypes.OffsetTypeAggregator, fftypes.SystemNamespace, aggregatorOffsetName).Return(&fftypes.Offset{
		Type:      fftypes.OffsetTypeAggregator,
		Namespace: fftypes.SystemNamespace,
		Name:      aggregatorOffsetName,
		Current:   12345,
	}, nil)
	mdi.On("GetEvents", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Event{}, nil)
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	err := ag.start()
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), ag.eventPoller.pollingOffset)
	ag.eventPoller.eventNotifier.newEvents <- 12345
	cancel()
	<-ag.eventPoller.closed
}

func TestProcessEventsNoopIncrement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	var runAsGroupFn func(context.Context) error
	mdi.On("UpdateOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mdi.On("RunAsGroup", mock.Anything, mock.MatchedBy(
		func(fn func(context.Context) error) bool {
			runAsGroupFn = fn
			return true
		},
	)).Return(nil, nil)
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))

	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageConfirmed, "ns1", fftypes.NewUUID())
	ev1.Sequence = 111
	ev2 := fftypes.NewEvent(fftypes.EventTypeMessageConfirmed, "ns1", fftypes.NewUUID())
	ev2.Sequence = 112
	ev3 := fftypes.NewEvent(fftypes.EventTypeMessageConfirmed, "ns1", fftypes.NewUUID())
	ev3.Sequence = 113
	_, err := ag.processEventRetryAndGroup([]*fftypes.Event{
		ev1, ev2, ev3,
	})
	runAsGroupFn(context.Background())
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestProcessEventCheckSequencedReadFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", fftypes.NewUUID())
	ev1.Sequence = 111
	_, err := ag.processEvents(context.Background(), []*fftypes.Event{ev1})
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessEventIgnoredTypeConfirmed(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageConfirmed, "ns1", fftypes.NewUUID())
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteDataNotAvailableBlock(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(false, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(nil, nil)
	mdi.On("UpsertBlocked", mock.Anything, mock.Anything, false).Return(nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteDataAvailableBlocked(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		Message: fftypes.NewUUID(), // Another message, not us
	}, nil)

	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteWeUnblock(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil) // set confirmed
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)                // confirmed event
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{
		{ID: fftypes.NewUUID()},
	}, nil)
	mdi.On("UpdateBlocked", mock.Anything, uuidMatches(blockedID), mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.True(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteUpdateBlockedFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil) // set confirmed
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)                // confirmed event
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{
		{ID: fftypes.NewUUID()},
	}, nil)
	mdi.On("UpdateBlocked", mock.Anything, uuidMatches(blockedID), mock.Anything).Return(fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteFailInsertUnblockEvent(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil).Once()
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{
		{ID: fftypes.NewUUID()},
	}, nil)
	mdi.On("UpdateBlocked", mock.Anything, uuidMatches(blockedID), mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteWeUnblockLookAheadOptimized(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil) // set confirmed
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)                // confirmed event
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{
		{ID: msgID2},
	}, nil)
	mdi.On("UpdateBlocked", mock.Anything, uuidMatches(blockedID), mock.Anything).Return(nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll) // No need to re-poll, as we've got the interesting event in our lookahead buffer
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessDataArrivedWeUnblockNoFutureMsgsFromData(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Message{msg}, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil) // set confirmed
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)                // confirmed event
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{}, nil)
	mdi.On("DeleteBlocked", mock.Anything, uuidMatches(blockedID)).Return(nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeDataArrivedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll) // No need to re-poll, as we've got the interesting event in our lookahead buffer
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteGetBlockedByContextFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(nil, fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll) // No need to re-poll, as we've got the interesting event in our lookahead buffer
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteBlockedGetMessageRefsFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      fftypes.NewUUID(),
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{}, fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll) // No need to re-poll, as we've got the interesting event in our lookahead buffer
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteBlockedDeleteBlockedFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      fftypes.NewUUID(),
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{}, nil)
	mdi.On("DeleteBlocked", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll) // No need to re-poll, as we've got the interesting event in our lookahead buffer
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessMsgCompleteDeleteBlockedFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	blockedID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(&fftypes.Blocked{
		ID:      blockedID,
		Message: msgID, // Blocked by us
	}, nil)
	mdi.On("UpdateMessage", mock.Anything, uuidMatches(msgID), mock.Anything).Return(nil) // set confirmed
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(nil)                // confirmed event
	mdi.On("GetMessageRefs", mock.Anything, mock.Anything).Return([]*fftypes.MessageRef{}, nil)
	mdi.On("DeleteBlocked", mock.Anything, uuidMatches(blockedID)).Return(fmt.Errorf("pop"))
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{
		*msgID2: []*fftypes.Event{{Type: fftypes.EventTypeMessageConfirmed}},
	}, ev1)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessEvent(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msgID := fftypes.NewUUID()
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        msgID,
			Namespace: "ns1",
			Context:   "context1",
		},
	}
	mdi.On("GetMessageByID", mock.Anything, mock.Anything).Return(msg, nil)
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(false, nil)
	mdi.On("GetBlockedByContext", mock.Anything, "ns1", "context1", (*fftypes.UUID)(nil)).Return(nil, nil)
	mdi.On("UpsertBlocked", mock.Anything, mock.Anything, false).Return(nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeMessageSequencedBroadcast, "ns1", msgID)
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessEventDataArrivedNoMsgs(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Message{}, nil)
	ev1 := fftypes.NewEvent(fftypes.EventTypeDataArrivedBroadcast, "ns1", fftypes.NewUUID())
	ev1.Sequence = 111
	repoll, err := ag.processEvent(context.Background(), eventsByRef{}, ev1)
	assert.False(t, repoll)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ag.eventPoller.pollingOffset)
	mdi.AssertExpectations(t)
}

func TestProcessDataArrivedError(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeDataArrivedBroadcast}
	repoll, err := ag.processDataArrived(context.Background(), "ns1", eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestProcessDataCompleteMsgInLookahead(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Message{msg}, nil)
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeDataArrivedBroadcast}
	repoll, err := ag.processDataArrived(context.Background(), "ns1", eventsByRef{
		*msg.Header.ID: []*fftypes.Event{
			{Type: fftypes.EventTypeMessageSequencedBroadcast},
		},
	}, ev)
	assert.False(t, repoll)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestProcessDataCompleteMsgNotPinnedYet(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Message{msg}, nil)
	mdi.On("GetEvents", mock.Anything, mock.Anything).Return([]*fftypes.Event{}, nil)
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeDataArrivedBroadcast}
	repoll, err := ag.processDataArrived(context.Background(), "ns1", eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestProcessDataCompleteDrivesMsgGetEventsFailure(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("GetMessagesForData", mock.Anything, mock.Anything, mock.Anything).Return([]*fftypes.Message{msg}, nil)
	mdi.On("GetEvents", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeDataArrivedBroadcast}
	repoll, err := ag.processDataArrived(context.Background(), "ns1", eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestCheckMessageCompleteDataAvailFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(false, fmt.Errorf("pop"))
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeMessageSequencedBroadcast, Reference: msg.Header.ID}
	repoll, err := ag.checkMessageComplete(context.Background(), msg, eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestCheckMessageCompleteUpdateFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeMessageSequencedBroadcast, Reference: msg.Header.ID}
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, mock.Anything, mock.Anything, (*fftypes.UUID)(nil)).Return(nil, nil)
	mdi.On("UpdateMessage", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	repoll, err := ag.checkMessageComplete(context.Background(), msg, eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestCheckMessageCompleteInsertEventFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeMessageSequencedBroadcast, Reference: msg.Header.ID}
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, mock.Anything, mock.Anything, (*fftypes.UUID)(nil)).Return(nil, nil)
	mdi.On("UpdateMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mdi.On("UpsertEvent", mock.Anything, mock.Anything, false).Return(fmt.Errorf("pop"))
	repoll, err := ag.checkMessageComplete(context.Background(), msg, eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestCheckMessageCompleteInsertBlockEventFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeMessageSequencedBroadcast, Reference: msg.Header.ID}
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(false, nil)
	mdi.On("GetBlockedByContext", mock.Anything, mock.Anything, mock.Anything, (*fftypes.UUID)(nil)).Return(nil, nil)
	mdi.On("UpsertBlocked", mock.Anything, mock.Anything, false).Return(fmt.Errorf("pop"))
	repoll, err := ag.checkMessageComplete(context.Background(), msg, eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
}

func TestCheckMessageCompleteSystemHandlerFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ag := newAggregator(ctx, mdi, mbm, newEventNotifier(ctx))
	msg := &fftypes.Message{
		Header: fftypes.MessageHeader{
			ID:        fftypes.NewUUID(),
			Namespace: fftypes.SystemNamespace,
			Context:   fftypes.SystemContext,
			Topic:     fftypes.SystemTopicBroadcastDatatype,
		},
	}
	ev := &fftypes.Event{ID: fftypes.NewUUID(), Type: fftypes.EventTypeMessageSequencedBroadcast, Reference: msg.Header.ID}
	mdi.On("CheckDataAvailable", mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("GetBlockedByContext", mock.Anything, mock.Anything, mock.Anything, (*fftypes.UUID)(nil)).Return(nil, nil)
	mbm.On("HandleSystemBroadcast", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	repoll, err := ag.checkMessageComplete(context.Background(), msg, eventsByRef{}, ev)
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")
	mdi.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestEventsByRefRemoveNoMatch(t *testing.T) {
	u := fftypes.NewUUID()
	ebr := eventsByRef{
		*u: []*fftypes.Event{
			{Type: fftypes.EventTypeMessageConfirmed, ID: u},
		},
	}
	assert.True(t, ebr.remove(*u))
	assert.False(t, ebr.remove(*u))
}