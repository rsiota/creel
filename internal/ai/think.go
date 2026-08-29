package ai

import "strings"

// Qwen / DeepSeek-style raw reasoning is often embedded in content as
// <think>...</think> when the provider does not emit a separate reasoning field.
const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// ThinkFilter peels <think>…</think> blocks out of a streamed content feed so
// the UI can render chain-of-thought dimmed and keep only the visible answer
// in StreamDelta.Content / the accumulated reply.
type ThinkFilter struct {
	inThink bool
	partial string // trailing prefix of an open/close tag split across chunks
}

// Feed consumes the next content chunk and returns the visible answer slice
// and any reasoning text that became complete in this chunk.
func (f *ThinkFilter) Feed(chunk string) (content, reasoning string) {
	if chunk == "" && f.partial == "" {
		return "", ""
	}
	s := f.partial + chunk
	f.partial = ""
	var contentB, reasonB strings.Builder
	for len(s) > 0 {
		if f.inThink {
			if i := strings.Index(s, thinkClose); i >= 0 {
				reasonB.WriteString(s[:i])
				s = s[i+len(thinkClose):]
				f.inThink = false
				continue
			}
			if n := incompleteSuffix(s, thinkClose); n > 0 {
				reasonB.WriteString(s[:len(s)-n])
				f.partial = s[len(s)-n:]
				break
			}
			reasonB.WriteString(s)
			break
		}
		if i := strings.Index(s, thinkOpen); i >= 0 {
			contentB.WriteString(s[:i])
			s = s[i+len(thinkOpen):]
			f.inThink = true
			continue
		}
		if n := incompleteSuffix(s, thinkOpen); n > 0 {
			contentB.WriteString(s[:len(s)-n])
			f.partial = s[len(s)-n:]
			break
		}
		contentB.WriteString(s)
		break
	}
	return contentB.String(), reasonB.String()
}

// Flush emits any buffered partial tag text. Call at end-of-stream so a
// truncated open tag is not lost (treated as visible content, or reasoning if
// we were already inside a think block).
func (f *ThinkFilter) Flush() (content, reasoning string) {
	if f.partial == "" {
		return "", ""
	}
	s := f.partial
	f.partial = ""
	if f.inThink {
		return "", s
	}
	return s, ""
}

// incompleteSuffix returns how many trailing bytes of s are a non-empty proper
// prefix of tag (so the next chunk might complete it). 0 means s can be emitted
// as-is.
func incompleteSuffix(s, tag string) int {
	max := len(tag) - 1
	if max > len(s) {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}

// StripThinkTags removes complete <think>…</think> blocks from a finished
// reply (for ExtractSQL / non-streaming paths). Incomplete tags are left as-is.
func StripThinkTags(s string) string {
	var b strings.Builder
	for {
		start := strings.Index(s, thinkOpen)
		if start < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:start])
		rest := s[start+len(thinkOpen):]
		end := strings.Index(rest, thinkClose)
		if end < 0 {
			// Unclosed: drop the rest (treat as reasoning, not answer).
			break
		}
		s = rest[end+len(thinkClose):]
	}
	return b.String()
}
