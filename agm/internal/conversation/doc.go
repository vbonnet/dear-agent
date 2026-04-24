// Package conversation provides JSONL conversation format support for AGM (Agent-Generic Model).
//
// # JSONL Schema v1.0
//
// The JSONL (JSON Lines) format stores conversations as newline-delimited JSON.
// Each line is a valid JSON object. The first line is the conversation header,
// and subsequent lines are messages.
//
// ## Conversation Header (Line 1)
//
//	{
//	  "schema_version": "1.0",
//	  "created_at": "2026-01-19T18:00:00Z",
//	  "model": "claude-sonnet-4-5",
//	  "agent": "claude",
//	  "total_messages": 10,
//	  "total_tokens": {
//	    "input_tokens": 1500,
//	    "output_tokens": 800
//	  }
//	}
//
// ## Message Format (Lines 2+)
//
//	{
//	  "timestamp": "2026-01-19T18:00:01Z",
//	  "role": "user",
//	  "agent": "claude",
//	  "content": [
//	    {"type": "text", "text": "Hello, world"}
//	  ]
//	}
//
//	{
//	  "timestamp": "2026-01-19T18:00:02Z",
//	  "role": "assistant",
//	  "agent": "claude",
//	  "content": [
//	    {"type": "text", "text": "Hi there!"}
//	  ],
//	  "usage": {
//	    "input_tokens": 5,
//	    "output_tokens": 2
//	  }
//	}
//
// ## Content Block Types
//
// ### Text Block
//
//	{"type": "text", "text": "Content here"}
//
// ### Image Block
//
//	{
//	  "type": "image",
//	  "source": {
//	    "type": "base64",
//	    "media_type": "image/png",
//	    "data": "iVBORw0KG..."
//	  }
//	}
//
// Or with URL:
//
//	{
//	  "type": "image",
//	  "source": {
//	    "type": "url",
//	    "media_type": "image/png",
//	    "url": "https://example.com/image.png"
//	  }
//	}
//
// ### Tool Use Block
//
//	{
//	  "type": "tool_use",
//	  "id": "tool_123",
//	  "name": "calculator",
//	  "input": {"expression": "2+2"}
//	}
//
// ### Tool Result Block
//
//	{
//	  "type": "tool_result",
//	  "tool_use_id": "tool_123",
//	  "content": "4"
//	}
//
// ## Multi-Agent Support
//
// Each message has an `agent` field to support conversations across multiple AI agents:
//
//	{"timestamp": "...", "role": "user", "agent": "claude", "content": [...]}
//	{"timestamp": "...", "role": "assistant", "agent": "claude", "content": [...]}
//	{"timestamp": "...", "role": "user", "agent": "gemini", "content": [...]}
//	{"timestamp": "...", "role": "assistant", "agent": "gemini", "content": [...]}
//
// ## Usage Example
//
//	// Parse JSONL file
//	conv, err := conversation.ParseJSONL("conversation.jsonl")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Access messages
//	for _, msg := range conv.Messages {
//	    fmt.Printf("[%s] %s: ", msg.Role, msg.Harness)
//	    for _, block := range msg.Content {
//	        if tb, ok := block.(conversation.TextBlock); ok {
//	            fmt.Println(tb.Text)
//	        }
//	    }
//	}
//
//	// Write JSONL file
//	err = conversation.WriteJSONL("output.jsonl", conv)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Convert Claude HTML to JSONL
//	err = conversation.ConvertHTMLToJSONL("conversation.html", "conversation.jsonl")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Validate conversation
//	err = conversation.ValidateConversation(conv)
//	if err != nil {
//	    log.Fatal(err)
//	}
package conversation
