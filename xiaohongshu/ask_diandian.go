package xiaohongshu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// 设计是：问点点复用打开页面那一套，打开点点（/ai_chat），带完整笔记链接提问，
// 等回答说完再读出文字。短链点点不认，调用方要先换成完整链接。
const (
	diandianInputSelector  = ".ai-chat-welcome__input textarea"
	diandianAnswerSelector = ".ai-message .markdown-block"
	diandianFinishedClass  = ".ai-message.ai-message-finished"
	diandianAnswerTimeout  = 2 * time.Minute
)

type AskDiandianAction struct {
	page *rod.Page
}

func NewAskDiandianAction(page *rod.Page) *AskDiandianAction {
	return &AskDiandianAction{page: page}
}

// Ask 在点点里发"笔记链接 + 问题"，返回点点的回答文字。
func (a *AskDiandianAction) Ask(ctx context.Context, noteURL, question string) (string, error) {
	page := a.page.Context(ctx).Timeout(diandianAnswerTimeout + time.Minute)

	if err := page.Navigate(webURL("/ai_chat")); err != nil {
		return "", fmt.Errorf("打开点点失败: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("等待点点页面加载失败: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterNavigate)

	inputBox, err := page.Timeout(20 * time.Second).Element(diandianInputSelector)
	if err != nil {
		return "", fmt.Errorf("未找到点点输入框（可能未登录或页面改版）: %w", err)
	}
	inputBox = inputBox.CancelTimeout()
	// 输入框要收到一次真实点击才能输入（只聚焦不行）；上面盖着占位提示层，
	// 评论那种带落点检查的点击会一直卡住，所以按坐标点输入框中心。
	shape, err := inputBox.Shape()
	if err != nil || shape.Box() == nil {
		return "", fmt.Errorf("无法定位点点输入框: %v", err)
	}
	box := shape.Box()
	if err := humanize.ClickAt(page, proto.Point{X: box.X + box.Width/2, Y: box.Y + box.Height/2}); err != nil {
		return "", fmt.Errorf("无法点击点点输入框: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterClick)

	// 链接和问题一起整段粘贴：输入后输入框会重新渲染，逐字输入会拿着失效的元素一直等。
	if err := page.InsertText(noteURL + " " + question); err != nil {
		return "", fmt.Errorf("无法输入问题: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterType)

	if err := page.Keyboard.Type(input.Enter); err != nil {
		return "", fmt.Errorf("发送问题失败: %w", err)
	}
	logrus.Infof("已向点点提问: %s", noteURL)

	return waitDiandianAnswer(ctx, page)
}

// waitDiandianAnswer 等回答带上 ai-message-finished 标记，再读最后一条回答的文字。
func waitDiandianAnswer(ctx context.Context, page *rod.Page) (string, error) {
	deadline := time.Now().Add(diandianAnswerTimeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if finished, _, _ := page.Has(diandianFinishedClass); finished {
			result, err := page.Eval(`(s) => {
				const blocks = document.querySelectorAll(s);
				return blocks.length ? blocks[blocks.length - 1].innerText : '';
			}`, diandianAnswerSelector)
			if err != nil {
				return "", fmt.Errorf("读取点点回答失败: %w", err)
			}
			answer := strings.TrimSpace(result.Value.String())
			if answer == "" {
				return "", fmt.Errorf("点点回答为空")
			}
			return answer, nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("等待点点回答超时（%s）", diandianAnswerTimeout)
}
