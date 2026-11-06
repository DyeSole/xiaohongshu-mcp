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

	var answer string
	err = withDiandian(ctx, func(action *xiaohongshu.AskDiandianAction) error {
		var askErr error
		answer, askErr = action.Ask(ctx, noteURL, question)
		return askErr
	})
	if err != nil {
		return nil, fmt.Errorf("问点点失败: %w", err)
	}
	return &AskDiandianResponse{NoteURL: noteURL, Question: question, Answer: answer}, nil
}

// FindDiandianRequest 让点点按关键词找笔记。
type FindDiandianRequest struct {
	Keyword string `json:"keyword"`
}

// FindDiandianResponse 点点的回答和参考来源里的笔记。
type FindDiandianResponse struct {
	Keyword string                       `json:"keyword"`
	Answer  string                       `json:"answer"`
	Sources []xiaohongshu.DiandianSource `json:"sources"`
}

// FindDiandianNotes 设计是：rednote 搜索改成问点点找笔记，顺带读回参考来源。
func (s *XiaohongshuService) FindDiandianNotes(ctx context.Context, req FindDiandianRequest) (*FindDiandianResponse, error) {
	keyword := strings.TrimSpace(req.Keyword)
	if keyword == "" {
		return nil, fmt.Errorf("缺少关键词")
	}
	var answer string
	var sources []xiaohongshu.DiandianSource
	err := withDiandian(ctx, func(action *xiaohongshu.AskDiandianAction) error {
		var findErr error
		answer, sources, findErr = action.FindNotes(ctx, keyword)
		return findErr
	})
	if err != nil {
		return nil, fmt.Errorf("点点找笔记失败: %w", err)
	}
	if sources == nil {
		sources = []xiaohongshu.DiandianSource{}
	}
	return &FindDiandianResponse{Keyword: keyword, Answer: answer, Sources: sources}, nil
}

// withDiandian 点点一次只问一个，两次之间至少隔 diandianMinInterval。
func withDiandian(ctx context.Context, run func(*xiaohongshu.AskDiandianAction) error) error {
	diandianGate.Lock()
	defer diandianGate.Unlock()
	if wait := diandianMinInterval - time.Since(diandianGate.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer func() { diandianGate.last = time.Now() }()

	b := newBrowser()
	defer b.Close()
	page := b.NewPage()
	defer page.Close()
	return run(xiaohongshu.NewAskDiandianAction(page))
}
