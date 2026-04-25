package stream

import "testing"

func TestDetectQuickRepliesReturnsShiFouForApprovalPrompt(t *testing.T) {
	replies := DetectQuickReplies("Need approval. Continue? [y/n]")
	if len(replies) != 2 || replies[0] != "是" || replies[1] != "否" {
		t.Fatalf("DetectQuickReplies() = %#v, want 是/否", replies)
	}
}

func TestDetectQuickRepliesReturnsNilForNormalOutput(t *testing.T) {
	replies := DetectQuickReplies("build finished successfully")
	if len(replies) != 0 {
		t.Fatalf("DetectQuickReplies() = %#v, want empty", replies)
	}
}
