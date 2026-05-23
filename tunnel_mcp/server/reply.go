package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Shaik-Sirajuddin/memory/mcp/store/message"
	"github.com/google/uuid"
)

type replyService struct {
	msgStore     message.MessageStore
	onNewMessage func(context.Context, string, string)
}

func newReplyService(msgStore message.MessageStore, onNewMessage func(context.Context, string, string)) *replyService {
	return &replyService{
		msgStore:     msgStore,
		onNewMessage: onNewMessage,
	}
}

func (r *replyService) SendReply(ctx context.Context, msg *message.Message, fromAgentID string) error {
	if msg == nil || !msg.ShouldReply {
		return nil
	}
	if r.msgStore == nil {
		return fmt.Errorf("message store is unavailable")
	}
	reply := &message.Message{
		ID:          uuid.NewString(),
		To:          msg.From,
		From:        fromAgentID,
		FromSpec:    message.SpecOmni,
		ToSpec:      message.SpecOmni,
		RequestType: msg.RequestType,
		IsResponse:  true,
		ShouldReply: false,
		RespondedTo: msg.ID,
		Prompt:      replyPrompt(msg, fromAgentID),
		Refs:        replyRefs(msg, fromAgentID),
		Workspace:   msg.Workspace,
		Status:      message.StatusInQueue,
		SentTime:    time.Now().UnixMilli(),
	}
	if err := r.msgStore.InsertMessage(ctx, reply); err != nil {
		return err
	}
	if r.onNewMessage != nil {
		r.onNewMessage(ctx, fromAgentID, msg.From)
	}
	return nil
}

func replyPrompt(msg *message.Message, fromAgentID string) string {
	return fmt.Sprintf("This is a reply to an earlier message.\n\nOriginal message id: %s\nReplying agent: %s\nOriginal sender: %s", msg.ID, fromAgentID, msg.From)
}

func replyRefs(msg *message.Message, fromAgentID string) string {
	fields := map[string]json.RawMessage{}
	if strings.TrimSpace(msg.Refs) != "" {
		_ = json.Unmarshal([]byte(msg.Refs), &fields)
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	putReplyRef(fields, "author", "tunnel-mcp")
	putReplyRef(fields, "author_agent_name", fromAgentID)
	putReplyRef(fields, "reply_to_message_id", msg.ID)
	putReplyRef(fields, "original_sender", msg.From)
	putReplyRef(fields, "author_workspace", msg.Workspace)

	out, err := json.Marshal(fields)
	if err != nil {
		return `{"author":"tunnel-mcp"}`
	}
	return string(out)
}

func putReplyRef(fields map[string]json.RawMessage, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	fields[key] = encoded
}
