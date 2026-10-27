package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// 设计是：问点点一次只问一个，两次之间至少隔一段时间，别被当成刷接口。
const diandianMinInterval = 15 * time.Second

var diandianGate struct {
	sync.Mutex
	last time.Time
}

const defaultDiandianQuestion = "这篇笔记讲了啥？"

// AskDiandianRequest 问点点请求：url 可以是短链、分享文案或完整链接；也可以直接给 feed_id + xsec_token。
type AskDiandianRequest struct {
	URL       string `json:"url,omitempty"`
	FeedID    string `json:"feed_id,omitempty"`
	XsecToken string `json:"xsec_token,omitempty"`
	Question  string `json:"question,omitempty"`
}

// AskDiandianResponse 问点点的回答。
type AskDiandianResponse struct {
	NoteURL  string `json:"note_url"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// AskDiandian 带着笔记去问点点，返回回答文字。
func (s *XiaohongshuService) AskDiandian(ctx context.Context, req AskDiandianRequest) (*AskDiandianResponse, error) {
	noteURL, err := xiaohongshu.NormalizeNoteURL(req.URL, req.FeedID, req.XsecToken, xiaohongshu.ResolveShortLink)
	if err != nil {
		return nil, err
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = defaultDiandianQuestion
	}

	diandianGate.Lock()
	defer diandianGate.Unlock()
	if wait := diandianMinInterval - time.Since(diandianGate.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { diandianGate.last = time.Now() }()

	b := newBrowser()
	defer b.Close()
	page := b.NewPage()
	defer page.Close()

	answer, err := xiaohongshu.NewAskDiandianAction(page).Ask(ctx, noteURL, question)
	if err != nil {
		return nil, fmt.Errorf("问点点失败: %w", err)
	}
	return &AskDiandianResponse{NoteURL: noteURL, Question: question, Answer: answer}, nil
}
