//go:build integration

package engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Shaik-Sirajuddin/memory/mcp/engine"
	"github.com/Shaik-Sirajuddin/memory/mcp/store/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessingEngineIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("Message Callback Cycle", func(t *testing.T) {
		msgStore := message.WithTestDB(t)
		omni := newRecordingOmniCLI()
		processor := engine.New(msgStore, engine.WithTestBinary(omni))
		firstMessage := testEngineMessage("msg-engine-1", "tengine", "head", 1000, false)
		secondMessage := testEngineMessage("msg-engine-2", "tengine", "head", 2000, false)
		insertEngineMessages(t, ctx, msgStore, firstMessage, secondMessage)

		processor.MessageArrived(ctx, "head", "tengine")

		require.Equal(t, "tengine", omni.waitForExec(t), "First ExecInSession call should target the queued agent")
		assert.Equal(t, []string{"tengine"}, omni.execCalls(), "ExecInSession calls should include the first dispatch")

		processor.AgentCallback(ctx, engine.AgentCallbackRequest{
			Source:  engine.MessageRef{MessageID: firstMessage.ID},
			AgentID: "tengine",
		}, false)

		require.Equal(t, "tengine", omni.waitForExec(t), "Second ExecInSession call should target the same agent after callback")

		delivered, err := msgStore.GetMessage(ctx, firstMessage.ID)
		require.NoError(t, err, "GetMessage should load the completed source message")
		assert.Equal(t, message.StatusDelivered, delivered.Status, "AgentCallback should mark the source message delivered")
		assert.NotNil(t, delivered.DeliveryTime, "AgentCallback should set delivery time on the source message")

		// second message is picked and marked queued before the second exec fires
		queued, err := msgStore.GetMessage(ctx, secondMessage.ID)
		require.NoError(t, err, "GetMessage should load the next message")
		assert.Equal(t, message.Status("queued"), queued.Status, "pickNextMessages should mark the second message queued")
		assert.Equal(t, []string{"tengine", "tengine"}, omni.execCalls(), "ExecInSession calls should include each command cycle dispatch")
	})
}

type recordingOmniCLI struct {
	mu    sync.Mutex
	execs []string
	ch    chan string
}

func newRecordingOmniCLI() *recordingOmniCLI {
	return &recordingOmniCLI{
		ch: make(chan string, 10),
	}
}

func (c *recordingOmniCLI) ExecInSession(_ context.Context, agentID, _, _ string) error {
	c.mu.Lock()
	c.execs = append(c.execs, agentID)
	c.mu.Unlock()
	c.ch <- agentID
	return nil
}

func (c *recordingOmniCLI) GetPromptState(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (c *recordingOmniCLI) waitForExec(t *testing.T) string {
	t.Helper()
	select {
	case agentID := <-c.ch:
		return agentID
	case <-time.After(2 * time.Second):
		require.FailNow(t, "ExecInSession should be called before timeout")
		return ""
	}
}

func (c *recordingOmniCLI) execCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	calls := make([]string, len(c.execs))
	copy(calls, c.execs)
	return calls
}

func testEngineMessage(id, to, by string, sentTime int64, shouldReply bool) *message.Message {
	return &message.Message{
		ID:          id,
		To:          to,
		From:        by,
		FromSpec:    message.SpecOmni,
		ToSpec:      message.SpecOmni,
		RequestType: message.RequestTypeExecute,
		ShouldReply: shouldReply,
		Prompt:      "execute test command",
		Refs:        "{}",
		Status:      message.StatusInQueue,
		SentTime:    sentTime,
	}
}

func insertEngineMessages(t *testing.T, ctx context.Context, msgStore message.MessageStore, msgs ...*message.Message) {
	t.Helper()
	for _, msg := range msgs {
		require.NoError(t, msgStore.InsertMessage(ctx, msg), "InsertMessage should seed engine integration test data")
	}
}
