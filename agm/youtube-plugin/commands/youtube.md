---
content-hash: PLACEHOLDER
description: Extract transcript from a YouTube video. Use when the user provides a YouTube URL or video ID and wants the content/transcript.
argument-hint: "{youtube-url-or-video-id}"
allowed-tools: Bash(yt-dlp:*), Bash(sed:*), Bash(awk:*), Bash(cat:*), Bash(rm:*), Bash(mkdir:*), Bash(ls:*), Bash(wc:*), Read, Write(/tmp/**)
---

# YouTube Transcript Extraction

I'll extract the transcript from a YouTube video.

**Step 1: Parse and validate input**

Parse $ARGUMENTS to extract the YouTube URL or video ID. Do NOT use bash conditionals — handle all logic in your reasoning layer.

**1.1: Check for empty input**
- If $ARGUMENTS is empty or whitespace only:
  - Show error: "No YouTube URL or video ID provided"
  - Show message: "Usage: /youtube {youtube-url-or-video-id}"
  - Exit gracefully

**1.2: Determine URL and video ID**
- If $ARGUMENTS is an 11-character alphanumeric string (video ID only):
  - Construct full URL: `https://www.youtube.com/watch?v={VIDEO_ID}`
  - Store as VIDEO_URL and VIDEO_ID
- If $ARGUMENTS contains a YouTube URL:
  - From `youtube.com/watch?v=XXXXXXXXXXX` — extract the `v` parameter as VIDEO_ID
  - From `youtu.be/XXXXXXXXXXX` — extract the path segment as VIDEO_ID
  - From `youtube.com/shorts/XXXXXXXXXXX` — extract the path segment as VIDEO_ID
  - From `youtube.com/live/XXXXXXXXXXX` — extract the path segment as VIDEO_ID
  - Strip any query parameters or timestamps after the video ID
  - Store the original URL as VIDEO_URL
- If $ARGUMENTS doesn't match any known format:
  - Show error: "Invalid YouTube URL or video ID"
  - Show message: "Accepted formats: YouTube URL, youtu.be short link, or 11-char video ID"
  - Exit gracefully

**Note**: Parse the URL in your reasoning layer. Do NOT use bash if/elif/else conditionals.

**Step 2: Create temp directory**

Run: `mkdir -p /tmp/yt-transcript`

**Step 3: Fetch video title**

Run: `yt-dlp --print title "VIDEO_URL"`

- If the command succeeds: store the output as VIDEO_TITLE
- If the command fails with "Private video" or "Video unavailable":
  - Show error: "Video is private or unavailable"
  - Exit gracefully
- If the command fails with "not a valid URL" or similar:
  - Show error: "Invalid YouTube URL: VIDEO_URL"
  - Exit gracefully
- If yt-dlp is not found:
  - Show error: "yt-dlp not found. Install with: brew install yt-dlp"
  - Exit gracefully

**Step 4: Fetch subtitles**

Run: `yt-dlp --write-auto-sub --sub-lang en --skip-download --sub-format vtt -o "/tmp/yt-transcript/%(id)s" "VIDEO_URL"`

- If the command succeeds: continue to Step 5
- If the output contains "no subtitles" or indicates no subs available:
  - Show message: "No English subtitles available for this video"
  - Show the video title that was fetched
  - Show suggestion: "This video may not have captions, or captions may be in another language."
  - Exit gracefully

**Step 5: Locate the VTT file**

Run: `ls /tmp/yt-transcript/VIDEO_ID*.vtt`

- Store the first matching file path as VTT_FILE
- If no .vtt file found:
  - Show error: "Subtitle file not found after download"
  - Exit gracefully

**Step 6: Parse VTT to clean text**

Run the following sed + awk pipeline as a single command:

```
sed '/^WEBVTT/d; /^Kind:/d; /^Language:/d; /^NOTE/d; /^$/d; /^[0-9]*$/d; /-->/d; s/<[^>]*>//g; s/&nbsp;/ /g; s/&amp;/\&/g; s/&lt;/</g; s/&gt;/>/g' "VTT_FILE" | awk '!seen[$0]++' > "/tmp/yt-transcript/VIDEO_ID.txt"
```

This pipeline:
- Removes WEBVTT header, Kind/Language metadata, NOTE blocks
- Removes empty lines, numeric cue IDs, timestamp lines (containing -->)
- Strips HTML tags (YouTube auto-subs contain `<c>` tags)
- Decodes common HTML entities
- Deduplicates lines (YouTube auto-subs repeat lines across cues)

**Step 7: Get character count**

Run: `wc -c < /tmp/yt-transcript/VIDEO_ID.txt`

Store the trimmed output as CHAR_COUNT.

**Step 8: Read and present the transcript**

Use the Read tool to read `/tmp/yt-transcript/VIDEO_ID.txt`.

Present the output in this format:

```
**VIDEO_TITLE**
VIDEO_URL
CHAR_COUNT characters | Saved to /tmp/yt-transcript/VIDEO_ID.txt

---

TRANSCRIPT_CONTENT
```

**Step 9: Clean up VTT file**

Run: `rm -f "VTT_FILE"`

Keep the .txt file for later reference during the session.

**Error Handling**:
- If yt-dlp is not found: "Install yt-dlp: brew install yt-dlp"
- If video is private/unavailable: Show clear error with the URL
- If no English subs: Suggest the video may not have captions
- If VTT parsing produces empty output: Show warning that subtitles may be in a non-standard format
