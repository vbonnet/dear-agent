# ADR-002: Google Generative AI Go SDK Selection

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** Gemini Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

The Gemini Adapter needs to integrate with Google's Gemini 2.0 API. We must choose between available SDK options for making API calls.

**Key Requirements:**
- Reliable API integration
- Support for Gemini 2.0 models
- Conversation history management
- Idiomatic Go interface

**Constraints:**
- Must support chat sessions with history
- Must handle authentication via API key
- Should be officially supported by Google

## Decision Drivers

- **Official Support:** Prefer Google-maintained SDKs
- **Feature Completeness:** Support for Gemini 2.0 capabilities
- **Stability:** Well-tested, production-ready
- **Documentation:** Clear examples and API docs
- **Community:** Active maintenance and issue resolution
- **Future-Proofing:** Support for upcoming features (streaming, tools)

## Considered Options

### Option 1: Official Google Generative AI Go SDK (Chosen)
**Package:** `github.com/google/generative-ai-go/genai`

**Pros:**
- ✅ Officially maintained by Google
- ✅ Native Go implementation
- ✅ Full Gemini 2.0 support
- ✅ Built-in chat session management
- ✅ Idiomatic Go API
- ✅ Handles conversation history
- ✅ Active development
- ✅ Clear documentation

**Cons:**
- ⚠️ Relatively new (GA in 2024)
- ⚠️ Breaking changes possible (mitigated by versioning)

**Example Usage:**
```go
import (
    "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/option"
)

client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
model := client.GenerativeModel("gemini-2.0-flash-exp")

chat := model.StartChat()
chat.History = []genai.Content{
    {Role: "user", Parts: []genai.Part{genai.Text("Hello")}},
    {Role: "model", Parts: []genai.Part{genai.Text("Hi!")}},
}

resp, err := chat.SendMessage(ctx, genai.Text("How are you?"))
```

### Option 2: REST API via net/http
**Implementation:** Manual HTTP requests to Google AI REST API

**Pros:**
- ✅ No external dependencies
- ✅ Full control over requests
- ✅ Lightweight

**Cons:**
- ❌ Manual request/response handling
- ❌ No built-in chat session management
- ❌ Authentication complexity
- ❌ Error handling complexity
- ❌ Must maintain API compatibility manually
- ❌ More code to write and test

**Example Usage:**
```go
import "net/http"

req, _ := http.NewRequest("POST", "https://generativelanguage.googleapis.com/v1/models/gemini-2.0-flash-exp:generateContent", body)
req.Header.Set("x-goog-api-key", apiKey)
resp, err := client.Do(req)
// Manual JSON parsing...
```

### Option 3: Community SDK
**Potential:** Third-party Go wrapper for Gemini

**Pros:**
- ⚠️ May have convenience features

**Cons:**
- ❌ Not officially supported
- ❌ Maintenance risk
- ❌ Potential API drift
- ❌ Security concerns
- ❌ Unknown quality

**Evaluation:** No mature community SDKs found

## Decision Outcome

**Chosen Option:** **Option 1 - Official Google Generative AI Go SDK**

**Rationale:**
1. **Official Support:** Maintained by Google, guaranteed compatibility
2. **Chat Sessions:** Built-in `StartChat()` and history management
3. **Idiomatic Go:** Follows Go best practices and conventions
4. **Future-Proof:** Will receive updates for new Gemini features
5. **Production-Ready:** Used in official Google examples
6. **Simplicity:** Higher-level API reduces boilerplate code
7. **Error Handling:** Structured errors, no manual JSON parsing

**Key Features Leveraged:**
- `genai.NewClient()` - Client initialization with API key
- `client.GenerativeModel()` - Model selection
- `model.StartChat()` - Chat session creation
- `chat.History` - Conversation context management
- `chat.SendMessage()` - Message sending with automatic history

**Trade-offs Accepted:**
- SDK dependency (acceptable for official Google package)
- Slightly higher memory footprint than raw HTTP (acceptable)
- Potential breaking changes (mitigated by version pinning)

## Implementation Details

### Client Initialization
```go
import (
    "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/option"
)

func (a *GeminiAdapter) createClient(ctx context.Context) (*genai.Client, error) {
    client, err := genai.NewClient(ctx, option.WithAPIKey(a.apiKey))
    if err != nil {
        return nil, fmt.Errorf("failed to create Gemini client: %w", err)
    }
    return client, nil
}
```

### Model Selection
```go
model := client.GenerativeModel(a.modelName) // e.g., "gemini-2.0-flash-exp"
```

### Chat Session with History
```go
// Convert agent messages to Gemini format
var geminiHistory []*genai.Content
for _, msg := range history {
    role := "user"
    if msg.Role == RoleAssistant {
        role = "model"
    }
    geminiHistory = append(geminiHistory, &genai.Content{
        Role: role,
        Parts: []genai.Part{
            genai.Text(msg.Content),
        },
    })
}

// Start chat with history
chat := model.StartChat()
chat.History = geminiHistory

// Send new message
resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
```

### Response Extraction
```go
var responseText string
if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
    for _, part := range resp.Candidates[0].Content.Parts {
        if text, ok := part.(genai.Text); ok {
            responseText += string(text)
        }
    }
}
```

### Error Handling
```go
resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
if err != nil {
    return fmt.Errorf("failed to send message to Gemini: %w", err)
}
```

## Consequences

### Positive
- ✅ Reduced boilerplate code (vs manual HTTP)
- ✅ Built-in chat session management
- ✅ Official support and documentation
- ✅ Automatic request/response serialization
- ✅ Future feature support (streaming, tools)
- ✅ Idiomatic Go error handling

### Negative
- ⚠️ External dependency (mitigated: official Google package)
- ⚠️ SDK updates may introduce breaking changes (mitigated: version pinning)

### Neutral
- ℹ️ Must use Google's authentication patterns
- ℹ️ API surface larger than minimal HTTP wrapper
- ℹ️ Memory footprint slightly higher (acceptable)

## Validation

### Success Metrics
- [x] Client initialization successful with API key
- [x] Chat sessions work with conversation history
- [x] Messages sent and responses received
- [x] Error handling functional
- [x] Integration tests pass with real API

### Feature Support
- [x] Text generation
- [x] Conversation history
- [x] Role mapping (user/model)
- [ ] Streaming (deferred to V2)
- [ ] Function calling (deferred to V2)
- [ ] Vision input (deferred to V2)
- [ ] System instructions (deferred to V2)

## Dependency Management

### Version Pinning
```go
// go.mod
require (
    github.com/google/generative-ai-go v0.18.0
    google.golang.org/api v0.213.0
)
```

### Update Strategy
1. Monitor for security updates
2. Test new versions in development
3. Update go.mod with specific version
4. Run integration tests
5. Deploy if tests pass

## Future Enhancements (V2)

### Streaming Support
```go
// V2 feature using SDK streaming
stream := chat.SendMessageStream(ctx, genai.Text(message.Content))
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }
    fmt.Print(resp.Text())
}
```

### Function Calling
```go
// V2 feature using SDK tools
model.Tools = []*genai.Tool{
    {
        FunctionDeclarations: []*genai.FunctionDeclaration{
            {
                Name:        "get_weather",
                Description: "Get weather for location",
                Parameters: &genai.Schema{
                    Type: genai.TypeObject,
                    Properties: map[string]*genai.Schema{
                        "location": {Type: genai.TypeString},
                    },
                },
            },
        },
    },
}
```

### Vision Input
```go
// V2 feature using SDK multimodal
resp, err := chat.SendMessage(ctx,
    genai.Text("What's in this image?"),
    genai.ImageData("jpeg", imageBytes),
)
```

## Alternative Rejected: Why Not REST API?

**Code Comparison:**

**With SDK (Current):**
```go
// 5 lines
client, _ := genai.NewClient(ctx, option.WithAPIKey(apiKey))
model := client.GenerativeModel("gemini-2.0-flash-exp")
chat := model.StartChat()
chat.History = geminiHistory
resp, err := chat.SendMessage(ctx, genai.Text(message))
```

**With REST API (Rejected):**
```go
// 30+ lines
type Request struct {
    Contents []Content `json:"contents"`
}
type Content struct {
    Role  string `json:"role"`
    Parts []Part `json:"parts"`
}
type Part struct {
    Text string `json:"text"`
}

reqBody := Request{
    Contents: buildContents(history, message),
}
jsonData, _ := json.Marshal(reqBody)

url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent", model)
req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
req.Header.Set("x-goog-api-key", apiKey)
req.Header.Set("Content-Type", "application/json")

resp, err := http.DefaultClient.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()

var result Response
json.NewDecoder(resp.Body).Decode(&result)
// Extract text from result.Candidates[0].Content.Parts[0].Text
```

**Verdict:** SDK provides 6x less code with better error handling and type safety.

## References

- [Google Generative AI Go SDK](https://pkg.go.dev/github.com/google/generative-ai-go/genai)
- [Google AI API Documentation](https://ai.google.dev/docs)
- [SPEC.md](SPEC.md) - API integration requirements
- [ARCHITECTURE.md](ARCHITECTURE.md) - Integration points
